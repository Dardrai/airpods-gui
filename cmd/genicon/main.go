// Command genicon writes the application icon for packaging.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Dardrai/airpods-gui/internal/icon"
)

// hicolorSizes are the fixed sizes installed into the hicolor icon theme.
var hicolorSizes = []int{16, 22, 24, 32, 48, 64, 128, 256}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// writeHicolor writes the fixed sizes and the scalable icon into a hicolor theme tree.
func writeHicolor(dir string, glyph icon.Variant) error {
	for _, size := range hicolorSizes {
		name := fmt.Sprintf("%dx%d", size, size)
		path := filepath.Join(dir, name, "apps", "airpods-tray.png")
		if err := writeFile(path, icon.PNG(glyph, size)); err != nil {
			return err
		}
	}
	return writeFile(filepath.Join(dir, "scalable", "apps", "airpods-tray.svg"), []byte(icon.SVG()))
}

func main() {
	ico := flag.String("ico", "", "write the multi-size ICO here")
	png := flag.String("png", "", "write a 256 pixel PNG here")
	hicolor := flag.String("hicolor", "", "write the hicolor icon theme tree here")
	flag.Parse()
	glyph := icon.Variant{Level: icon.NoBadge, Tile: true}
	if *ico != "" {
		if err := os.WriteFile(*ico, icon.ICO(glyph), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	if *png != "" {
		if err := os.WriteFile(*png, icon.PNG(glyph, 256), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	if *hicolor != "" {
		if err := writeHicolor(*hicolor, glyph); err != nil {
			log.Fatal(err)
		}
	}
}
