package main
import (
"fmt"
"io"
"net/http"
"regexp"
"strings"
)
func main() {
resp, err := http.Get("https://mp.weixin.qq.com/s?__biz=MzA4NDQ1NjkzNQ==&mid=2650730444&idx=1&sn=2d7c5ed2def238c92a95c94291c9ddc1")
if err != nil { panic(err) }
defer resp.Body.Close()
b, _ := io.ReadAll(resp.Body)
s := string(b)

oldRe := regexp.MustCompile("(?is)<script\\b[^>]*\\bsrc\\s*=\\s*[\"'][^\"']*[\"'][^>]*>\\s*</script>")
newRe := regexp.MustCompile("(?is)<script\\b[^>]*\\bsrc\\s*=\\s*[\"'][^\"']*[\"'][^>]*>.*?</script>")
linkRe := regexp.MustCompile("(?is)<link\\b[^>]*\\brel\\s*=\\s*[\"'](modulepreload|preload|prefetch)[\"'][^>]*/?>")

oldMatches := oldRe.FindAllString(s, -1)
newMatches := newRe.FindAllString(s, -1)
linkMatches := linkRe.FindAllString(s, -1)

fmt.Printf("Old matched %d scripts\n", len(oldMatches))
fmt.Printf("New matched %d scripts\n", len(newMatches))
fmt.Printf("Link preload matched %d links\n", len(linkMatches))
    if len(newMatches) > len(oldMatches) {
       for _, m := range newMatches {
          if !strings.Contains(m, " ") { // just checking if it is empty... wait
          }
       }
    }
    // Also count all script src= tags
    allScr := regexp.MustCompile("(?is)<script[ \t\r\n][^>]*src[ \t\r\n]*=[^>]*>").FindAllString(s, -1)
    fmt.Printf("Total script with src: %d\n", len(allScr))
}
