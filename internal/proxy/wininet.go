//go:build windows

package proxy

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

var (
	wininet             = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOpt  = wininet.NewProc("InternetSetOptionW")

	user32              = syscall.NewLazyDLL("user32.dll")
	procSendMsgTimeout  = user32.NewProc("SendMessageTimeoutW")
)

const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
	hwndBroadcast                 = 0xFFFF
	wmSettingChange               = 0x001A
	smtoAbortIfHung               = 0x0002
)

type WinInetSnapshot struct {
	ProxyEnable   uint32
	ProxyServer   string
	ProxyOverride string
	AutoConfigURL string
}

func CaptureSystemProxy() (*WinInetSnapshot, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.QUERY_VALUE)
	if err != nil {
		return &WinInetSnapshot{}, nil
	}
	defer key.Close()

	enable, _, _ := key.GetIntegerValue("ProxyEnable")
	server, _, _ := key.GetStringValue("ProxyServer")
	override, _, _ := key.GetStringValue("ProxyOverride")
	autoConfig, _, _ := key.GetStringValue("AutoConfigURL")

	return &WinInetSnapshot{
		ProxyEnable:   uint32(enable),
		ProxyServer:   server,
		ProxyOverride: override,
		AutoConfigURL: autoConfig,
	}, nil
}

func ApplySystemProxy(proxyServer, proxyOverride string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer key.Close()

	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := key.SetStringValue("ProxyServer", proxyServer); err != nil {
		return err
	}

	if strings.TrimSpace(proxyOverride) == "" {
		proxyOverride = "<local>"
	}
	if err := key.SetStringValue("ProxyOverride", proxyOverride); err != nil {
		return err
	}

	key.DeleteValue("AutoConfigURL")
	return nil
}

func RestoreSystemProxy(snapshot *WinInetSnapshot) error {
	if snapshot == nil {
		return nil
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	key.SetDWordValue("ProxyEnable", snapshot.ProxyEnable)
	setOrDelete(key, "ProxyServer", snapshot.ProxyServer)
	setOrDelete(key, "ProxyOverride", snapshot.ProxyOverride)
	setOrDelete(key, "AutoConfigURL", snapshot.AutoConfigURL)

	return nil
}

func NotifyProxyChanged() {
	// Call InternetSetOption to notify WinINet of settings change
	procInternetSetOpt.Call(0, internetOptionSettingsChanged, 0, 0)
	procInternetSetOpt.Call(0, internetOptionRefresh, 0, 0)

	// Broadcast WM_SETTINGCHANGE to all top-level windows
	lParam, _ := syscall.UTF16PtrFromString("Internet Settings")
	var result uintptr
	procSendMsgTimeout.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(lParam)),
		smtoAbortIfHung,
		1000,
		uintptr(unsafe.Pointer(&result)),
	)
}

func setOrDelete(key registry.Key, name, value string) {
	if strings.TrimSpace(value) == "" {
		key.DeleteValue(name)
		return
	}
	key.SetStringValue(name, value)
}
