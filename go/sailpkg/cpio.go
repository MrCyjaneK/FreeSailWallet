package sailpkg

import (
	"encoding/hex"
	"fmt"
)

// readCPIO reads an SVR4 newc or crc cpio archive.
func readCPIO(data []byte) (map[string][]byte, error) {
	files := map[string][]byte{}
	i := 0
	for i+110 <= len(data) {
		magic := string(data[i : i+6])
		if magic != "070701" && magic != "070702" {
			return nil, fmt.Errorf("cpio magic %q at %d", magic, i)
		}
		mode, err := cpioHex(data[i+14 : i+22])
		if err != nil {
			return nil, err
		}
		filesize, err := cpioHex(data[i+54 : i+62])
		if err != nil {
			return nil, err
		}
		namesize, err := cpioHex(data[i+94 : i+102])
		if err != nil {
			return nil, err
		}
		if namesize < 1 {
			return nil, fmt.Errorf("cpio namesize %d", namesize)
		}
		nameStart := i + 110
		nameEnd := nameStart + namesize
		if nameEnd > len(data) {
			return nil, fmt.Errorf("cpio name past end")
		}
		rawName := data[nameStart : nameEnd-1]
		dataStart := (nameEnd + 3) &^ 3
		dataEnd := dataStart + filesize
		if dataEnd > len(data) {
			return nil, fmt.Errorf("cpio file past end")
		}
		name := string(rawName)
		if name == "TRAILER!!!" {
			return files, nil
		}
		// 0x8000 is a regular file. Other members are skipped.
		if mode&0xF000 == 0x8000 {
			body := make([]byte, filesize)
			copy(body, data[dataStart:dataEnd])
			files[name] = body
		}
		i = (dataEnd + 3) &^ 3
	}
	return nil, fmt.Errorf("cpio missing trailer")
}

func cpioHex(b []byte) (int, error) {
	var out [4]byte
	n, err := hex.Decode(out[:], b)
	if err != nil || n != 4 {
		return 0, fmt.Errorf("cpio field %q", b)
	}
	v := int(out[0])<<24 | int(out[1])<<16 | int(out[2])<<8 | int(out[3])
	if v < 0 {
		return 0, fmt.Errorf("cpio field overflow")
	}
	return v, nil
}
