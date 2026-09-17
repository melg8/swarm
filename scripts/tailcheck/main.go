package main

import (
	"fmt"
	"os"
)

// walkOne walks one 65536 block region from data[start:] and returns
// the consumed length or an error string.
func walkOne(data []byte, start int) (int, string) {
	pos := start
	for block := 0; block < 65536; block++ {
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
			for cell := 0; cell < 64; cell++ {
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
			return 0, fmt.Sprintf("bad type %d at %d", kind, pos-1)
		}
	}

	return pos - start, ""
}

func main() {
	for _, name := range os.Args[1:] {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		consumed, res := walkOne(data, 0)
		fmt.Printf("%s: first region %d bytes (%s)\n", name, consumed,
			res)
		for start := consumed; start < len(data); {
			n, res := walkOne(data, start)
			if n == 0 {
				fmt.Printf("  tail at %d: %s\n", start, res)

				break
			}
			fmt.Printf("  next region at %d: %d bytes (%s),"+
				" leftover %d\n", start, n, res,
				len(data)-start-n)
			start += n
		}
	}
}
