//go:build windows

package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

// CA holds a generated root CA certificate and private key.
// It can sign per-host leaf certificates for MITM interception.
type CA struct {
	cert       *x509.Certificate
	key        *ecdsa.PrivateKey
	tlsCert    tls.Certificate
	certPath   string
	thumbprint [20]byte
	installed  bool
}

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	procCertOpenStore      = crypt32.NewProc("CertOpenSystemStoreW")
	procCertAddEncoded     = crypt32.NewProc("CertAddEncodedCertificateToStore")
	procCertFindInStore    = crypt32.NewProc("CertFindCertificateInStore")
	procCertDeleteFromStore = crypt32.NewProc("CertDeleteCertificateFromStore")
	procCertCloseStore     = crypt32.NewProc("CertCloseStore")
)

const (
	certStoreAddReplaceExisting = 3
	certEncodingX509ASN         = 1
	certFindSHA1Hash            = 0x10000
)

// NewCA generates a new ECDSA P-256 root CA certificate and persists it under dataDir.
// If a CA already exists on disk, it is loaded instead.
func NewCA(dataDir string) (*CA, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	certPath := filepath.Join(dataDir, "wesaver_ca.crt")
	keyPath := filepath.Join(dataDir, "wesaver_ca.key")

	if ca, err := loadCA(certPath, keyPath); err == nil {
		// Check if CA cert is still valid (not expired or expiring within 30 days)
		if time.Now().Add(30 * 24 * time.Hour).Before(ca.cert.NotAfter) {
			return ca, nil
		}
		// CA expired or about to expire — regenerate
		os.Remove(certPath)
		os.Remove(keyPath)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "WeSaver Local CA",
			Organization: []string{"WeSaver"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(3 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("create CA cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	return loadCA(certPath, keyPath)
}

func loadCA(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	caCert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, err
	}

	priv, ok := tlsCert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unexpected key type")
	}

	return &CA{
		cert:       caCert,
		key:        priv,
		tlsCert:    tlsCert,
		certPath:   certPath,
		thumbprint: sha1.Sum(caCert.Raw),
	}, nil
}

// InstallToStore adds the CA certificate to the Windows "ROOT" trusted store.
// This triggers a Windows security prompt that the user must accept.
func (ca *CA) InstallToStore() error {
	hStore, _, err := procCertOpenStore.Call(0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("ROOT"))))
	if hStore == 0 {
		return fmt.Errorf("CertOpenSystemStore: %w", err)
	}
	defer procCertCloseStore.Call(hStore, 0)

	derBytes := ca.cert.Raw
	ret, _, err := procCertAddEncoded.Call(
		hStore,
		certEncodingX509ASN,
		uintptr(unsafe.Pointer(&derBytes[0])),
		uintptr(len(derBytes)),
		certStoreAddReplaceExisting,
		0,
	)
	if ret == 0 {
		return fmt.Errorf("CertAddEncodedCertificateToStore: %w", err)
	}

	ca.installed = true
	return nil
}

// UninstallFromStore removes the CA certificate from the Windows "ROOT" trusted store.
func (ca *CA) UninstallFromStore() error {
	if !ca.installed {
		return nil
	}

	hStore, _, err := procCertOpenStore.Call(0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("ROOT"))))
	if hStore == 0 {
		return fmt.Errorf("CertOpenSystemStore: %w", err)
	}
	defer procCertCloseStore.Call(hStore, 0)

	type cryptHashBlob struct {
		cbData uint32
		pbData uintptr
	}
	blob := cryptHashBlob{
		cbData: 20,
		pbData: uintptr(unsafe.Pointer(&ca.thumbprint[0])),
	}

	pCertCtx, _, _ := procCertFindInStore.Call(
		hStore,
		certEncodingX509ASN,
		0,
		certFindSHA1Hash,
		uintptr(unsafe.Pointer(&blob)),
		0,
	)
	if pCertCtx == 0 {
		ca.installed = false
		return nil
	}

	ret, _, err := procCertDeleteFromStore.Call(pCertCtx)
	if ret == 0 {
		return fmt.Errorf("CertDeleteCertificateFromStore: %w", err)
	}

	ca.installed = false
	return nil
}

// SignHost generates a TLS certificate for the given hostname, signed by this CA.
func (ca *CA) SignHost(host string) (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: host,
		},
		DNSNames:              []string{host},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &priv.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
