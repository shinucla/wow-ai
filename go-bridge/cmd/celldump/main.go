package main
import (
  "fmt"
  "github.com/chelinho139/wow-ai/go-bridge/internal/codec"
)
func main() {
  cells := codec.Encode(21, []byte("hello"))
  fmt.Println(cells[:32])
  for i,v := range cells[:16] {
    r,g,b := codec.CellColor(v)
    fmt.Printf("cell[%d]=%d rgb=(%.0f,%.0f,%.0f)\n", i, v, r,g,b)
  }
}
