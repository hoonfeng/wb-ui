package main

import (
    "bufio"
    "fmt"
    "os"
)

func main() {
    f, _ := os.Open("F:\\syproject\\wb-ui\\layout\\blockformattingcontext.go")
    defer f.Close()
    s := bufio.NewScanner(f)
    line := 0
    for s.Scan() {
        line++
        if line >= 83 && line <= 100 {
            fmt.Printf("L%d: %q\n", line, s.Text())
        }
    }
}
