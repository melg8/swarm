// The geodata block walk diagnostic: walks the 65536 blocks of one
// region file with the C1 block grammar (flat 1+2, complex 1+128,
// multilayer 1 + 64*(1+2*layers)), checks the block type invariant at
// every block boundary and reports the first desync offset together
// with the leftover byte count.
package main

import (
    "fmt"
    "os"
    "sort"
    "strconv"
    "strings"
)

func walk(data []byte, name string) {
    const blocks = 65536
    pos := 0
    typeCounts := map[byte]int{}
    layerHist := map[int]int{}
    desync := false
    for block := range blocks {
        if pos >= len(data) {
            fmt.Printf("%s: truncated at block %d offset %d\n", name,
                block, pos)

            return
        }
        if !desync && data[pos] > 2 {
            // The first boundary whose type byte is not 0/1/2.
            desync = true
            fmt.Printf("%s: desync before block %d: offset %d,"+
                " byte %d\n", name, block, pos, data[pos])
            lo := pos - 16
            if lo < 0 {
                lo = 0
            }
            hi := pos + 16
            if hi > len(data) {
                hi = len(data)
            }
            fmt.Printf("  context: % x\n", data[lo:hi])
        }
        kind := data[pos]
        typeCounts[kind]++
        pos++
        switch kind {
        case 0:
            pos += 2
        case 1:
            pos += 128
        case 2:
            for range 64 {
                if pos >= len(data) {
                    fmt.Printf("%s: truncated multilayer"+
                        " block %d\n", name, block)

                    return
                }
                layers := int(data[pos])
                layerHist[layers]++
                pos++
                if layers == 0 || layers > 125 {
                    if !desync {
                        desync = true
                        fmt.Printf("%s: bad layer count"+
                            " %d in block %d at offset"+
                            " %d\n", name, layers, block,
                            pos-1)
                    }
                    pos += 2
                    layers = 1
                }
                pos += layers * 2
            }
        default:
            // Unknown type: keep walking (the desync report fired).
            pos += 2
        }
    }
    fmt.Printf("%s: consumed %d of %d bytes, leftover %d, desync %t\n",
        name, pos, len(data), len(data)-pos, desync)
    fmt.Printf("  block types: %v\n", fmtCounts(typeCounts))
    if len(layerHist) > 0 {
        fmt.Printf("  layer counts: %v\n", fmtInts(layerHist))
    }
}

func fmtCounts(m map[byte]int) string {
    keys := make([]byte, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
    parts := make([]string, len(keys))
    for i, k := range keys {
        parts[i] = strconv.Itoa(int(k)) + ":" + strconv.Itoa(m[k])
    }

    return strings.Join(parts, " ")
}

func fmtInts(m map[int]int) string {
    keys := make([]int, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Ints(keys)
    parts := make([]string, 0, 12)
    for _, k := range keys {
        if len(parts) < 12 {
            parts = append(parts, strconv.Itoa(k)+":"+
                strconv.Itoa(m[k]))
        }
    }

    return strings.Join(parts, " ")
}

func main() {
    for _, name := range os.Args[1:] {
        data, err := os.ReadFile(name)
        if err != nil {
            fmt.Printf("%s: %v\n", name, err)

            continue
        }
        walk(data, name)
    }
}
