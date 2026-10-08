package main
import (
  "fmt"
  "image"
  "image/png"
  "os"
  "path/filepath"
  "github.com/chelinho139/wow-ai/go-bridge/internal/capture"
)
func main() {
  paths, _ := filepath.Glob("/mnt/c/Users/*/AppData/Roaming/wow-ai-bridge/probes/probe-i1-print.png")
  if len(paths) == 0 {
    fmt.Println("no probe png found")
    return
  }
  for _, p := range paths {
    f,_:=os.Open(p); img,_:=png.Decode(f); f.Close()
    b:=img.Bounds(); rgba:=image.NewRGBA(b)
    for y:=b.Min.Y;y<b.Max.Y;y++{for x:=b.Min.X;x<b.Max.X;x++{rgba.Set(x,y,img.At(x,y))}}
    fmt.Println("size", rgba.Rect.Dx(), rgba.Rect.Dy())
    // direct known good params from before - but file may be NEW 8px probe now!
    msg, err := capture.DecodeRGBAFloatSearch(rgba.Pix, rgba.Rect.Dx(), rgba.Rect.Dy(), rgba.Stride, true, 200, 48)
    fmt.Println("float", err, msg!=nil)
    if msg!=nil { fmt.Printf("id=%d len=%d\n", msg.ID, len(msg.Payload)) }
  }
}
