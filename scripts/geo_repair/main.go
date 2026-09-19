// The geodata concatenation repair: the owner pack ships a few region
// files as concatenations of several region streams (the first region
// is the named one, the tail holds appended neighbouring sea regions
// and a truncated fragment - the parse refuses them with the
// "trailing bytes" error). The tool walks the first region with the C1
// block grammar and truncates the file to it, keeping the original
// bytes beside the file as X_Y.l2j.orig. Regions whose first stream
// does not parse are reported and left alone.
package main

import (
    "fmt"
    "os"
    "path/filepath"
)

// walkOne walks one 65536 block region from data[start:] and returns
// the consumed length (0 with the reason on a refusal).
func walkOne(data []byte, start int) (int, string) {
    pos := start
    for range 65536 {
        if pos >= len(data) {
            return 0, "truncated"
        }
        kind := data[pos]
        pos++
        switch kind {
        case 0:
            pos += 2
        case 1:
            pos += 128
        case 2:
            for range 64 {
                if pos >= len(data) {
                    return 0, "truncated multilayer"
                }
                layers := int(data[pos])
                pos++
                if layers == 0 || layers > 125 {
                    return 0, fmt.Sprintf(
                        "bad layer count %d at %d",
                        layers, pos-1)
                }
                pos += layers * 2
            }
        default:
            return 0, fmt.Sprintf("bad block type %d at %d",
                kind, pos-1)
        }
    }

    return pos - start, ""
}

func main() {
    for _, name := range os.Args[1:] {
        data, err := os.ReadFile(name)
        if err != nil {
            fmt.Printf("%s: read %v\n", name, err)

            continue
        }
        consumed, res := walkOne(data, 0)
        if consumed == 0 || consumed == len(data) {
            fmt.Printf("%s: no repair (%s, %d bytes)\n", name,
                res, len(data))

            continue
        }
        orig := name + ".orig"
        if err := os.WriteFile(orig, data, 0o644); err != nil {
            fmt.Printf("%s: backup %v\n", name, err)

            continue
        }
        if err := os.WriteFile(name, data[:consumed], 0o644); err != nil {
            fmt.Printf("%s: truncate %v\n", name, err)

            continue
        }
        fmt.Printf("%s: truncated to the first region: %d of %d"+
            " bytes (dropped %d, backup %s)\n",
            filepath.Base(name), consumed, len(data),
            len(data)-consumed, filepath.Base(orig))
    }
}
