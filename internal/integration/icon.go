package integration

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sync"
)

//go:embed openidm_icon.jpg
var embeddedOpenIDMIconJPG []byte

var (
	decodeIconOnce sync.Once
	decodedIconImg image.Image
)

// loadEmbeddedSourceIcon decodifica em memória a imagem oficial do ícone do OpenIDM fornecida pelo usuário.
func loadEmbeddedSourceIcon() image.Image {
	decodeIconOnce.Do(func() {
		if len(embeddedOpenIDMIconJPG) > 0 {
			if img, err := jpeg.Decode(bytes.NewReader(embeddedOpenIDMIconJPG)); err == nil {
				decodedIconImg = img
			}
		}
	})
	return decodedIconImg
}

// RenderOpenIDMIcon renderiza o ícone oficial do OpenIDM na resolução solicitada com supersampling 4x4 (anti-aliasing)
// a partir da arte oficial embutida (fundo amarelo-ouro #F4B601 e símbolo de download preto #181818).
func RenderOpenIDMIcon(size int) *image.NRGBA {
	if size <= 0 {
		size = 64
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	src := loadEmbeddedSourceIcon()

	const samples = 4
	invSamples := 1.0 / float64(samples*samples)

	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var rSum, gSum, bSum, aSum float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					u := (float64(px) + (float64(sx)+0.5)/float64(samples)) / float64(size)
					v := (float64(py) + (float64(sy)+0.5)/float64(samples)) / float64(size)
					cr, cg, cb, ca := sampleOpenIDMPixel(src, u, v)
					rSum += cr * ca
					gSum += cg * ca
					bSum += cb * ca
					aSum += ca
				}
			}
			aAvg := aSum * invSamples
			if aAvg <= 0.001 {
				continue
			}
			rAvg := (rSum * invSamples) / aAvg
			gAvg := (gSum * invSamples) / aAvg
			bAvg := (bSum * invSamples) / aAvg

			img.SetNRGBA(px, py, color.NRGBA{
				R: clampByte(rAvg),
				G: clampByte(gAvg),
				B: clampByte(bAvg),
				A: clampByte(aAvg),
			})
		}
	}
	return img
}

// sampleOpenIDMPixel amostra o pixel normalizado (u, v) in [0, 1] da imagem oficial com bordas squircle suaves.
func sampleOpenIDMPixel(src image.Image, u, v float64) (r, g, b, a float64) {
	// Máscara squircle com cantos suavemente arredondados para bandeja do Windows, atalhos e barra de tarefas
	distOuter := sdRoundedBox(u-0.5, v-0.5, 0.495, 0.495, 0.14)
	if distOuter > 0.012 {
		return 0, 0, 0, 0
	}
	alphaOuter := smoothStep(0.010, -0.010, distOuter)

	if src != nil {
		bnds := src.Bounds()
		w := bnds.Dx()
		h := bnds.Dy()
		if w > 0 && h > 0 {
			sx := bnds.Min.X + int(u*float64(w))
			if sx >= bnds.Max.X {
				sx = bnds.Max.X - 1
			} else if sx < bnds.Min.X {
				sx = bnds.Min.X
			}
			sy := bnds.Min.Y + int(v*float64(h))
			if sy >= bnds.Max.Y {
				sy = bnds.Max.Y - 1
			} else if sy < bnds.Min.Y {
				sy = bnds.Min.Y
			}
			cr, cg, cb, _ := src.At(sx, sy).RGBA()
			return float64(cr >> 8), float64(cg >> 8), float64(cb >> 8), alphaOuter * 255.0
		}
	}

	// Fallback vetorial idêntico à arte oficial do OpenIDM (#F4B601 amarelo e #181818 símbolo)
	bgR, bgG, bgB := 244.0, 182.0, 1.0
	if evalOpenIDMSymbol(u, v) {
		bgR, bgG, bgB = 24.0, 24.0, 24.0
	}
	return bgR, bgG, bgB, alphaOuter * 255.0
}

// evalOpenIDMSymbol desenha a seta arredondada e bandeja em U caso o fallback vetorial seja utilizado.
func evalOpenIDMSymbol(u, v float64) bool {
	// Haste vertical da seta
	if sdRoundedBox(u-0.5, v-0.31, 0.098, 0.185, 0.035) <= 0 {
		return true
	}
	// Ponta triangular da seta apontando para baixo
	if v >= 0.44 && v <= 0.72 {
		progress := (0.72 - v) / (0.72 - 0.44)
		halfWidth := 0.025 + progress*0.215
		if math.Abs(u-0.5) <= halfWidth {
			return true
		}
	}
	// Bandeja inferior em formato de U
	if u >= 0.125 && u <= 0.875 && v >= 0.78 && v <= 0.875 {
		return true
	}
	if ((u >= 0.125 && u <= 0.235) || (u >= 0.765 && u <= 0.875)) && v >= 0.625 && v <= 0.875 {
		return true
	}
	return false
}

func sdRoundedBox(px, py, bx, by, r float64) float64 {
	qx := math.Abs(px) - bx + r
	qy := math.Abs(py) - by + r
	outX := math.Max(qx, 0.0)
	outY := math.Max(qy, 0.0)
	outside := math.Hypot(outX, outY)
	inside := math.Min(math.Max(qx, qy), 0.0)
	return outside + inside - r
}

func smoothStep(edge0, edge1, x float64) float64 {
	t := (x - edge0) / (edge1 - edge0)
	if t < 0.0 {
		t = 0.0
	} else if t > 1.0 {
		t = 1.0
	}
	return t * t * (3.0 - 2.0*t)
}

func clampByte(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// RenderOpenIDMIconPNG gera um arquivo PNG em memória na resolução solicitada.
func RenderOpenIDMIconPNG(size int) ([]byte, error) {
	img := RenderOpenIDMIcon(size)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderOpenIDMIconDataURL retorna o ícone oficial codificado como Data URL (data:image/png;base64,...) para páginas HTML.
func RenderOpenIDMIconDataURL(size int) string {
	pngBytes, err := RenderOpenIDMIconPNG(size)
	if err != nil || len(pngBytes) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
}

// encodeBMPDIBFrame codifica uma imagem NRGBA como um quadro DIB 32-bit BGRA bottom-up + máscara AND 1-bit
// para máxima compatibilidade com Windows Explorer, LoadIconW, rsrc e go-winres.
func encodeBMPDIBFrame(img *image.NRGBA) []byte {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	// Linha da máscara AND 1-bpp deve ser alinhada em múltiplos de 4 bytes (32 bits)
	andRowBytes := ((w + 31) / 32) * 4
	xorSize := w * h * 4
	andSize := andRowBytes * h
	totalSize := 40 + xorSize + andSize

	buf := make([]byte, totalSize)
	// BITMAPINFOHEADER (40 bytes)
	binary.LittleEndian.PutUint32(buf[0:4], 40)              // biSize
	binary.LittleEndian.PutUint32(buf[4:8], uint32(w))       // biWidth
	binary.LittleEndian.PutUint32(buf[8:12], uint32(h*2))    // biHeight (dobrado para incluir XOR + AND)
	binary.LittleEndian.PutUint16(buf[12:14], 1)             // biPlanes
	binary.LittleEndian.PutUint16(buf[14:16], 32)            // biBitCount (32-bpp BGRA)
	binary.LittleEndian.PutUint32(buf[16:20], 0)             // biCompression (BI_RGB)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(xorSize+andSize))

	xorOffset := 40
	andOffset := 40 + xorSize

	// Quadros DIB em ICO são armazenados de baixo para cima (bottom-up)
	for y := 0; y < h; y++ {
		srcY := h - 1 - y
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, srcY)
			p := xorOffset + (y*w+x)*4
			buf[p+0] = c.B
			buf[p+1] = c.G
			buf[p+2] = c.R
			buf[p+3] = c.A

			// Se totalmente transparente, marca bit=1 na máscara AND
			if c.A < 16 {
				byteIdx := andOffset + y*andRowBytes + (x / 8)
				bitIdx := uint(7 - (x % 8))
				buf[byteIdx] |= 1 << bitIdx
			}
		}
	}

	return buf
}

// GenerateOpenIDMICO gera um arquivo .ico multi-resolução (256, 128, 64, 48, 32, 16 px) completo.
func GenerateOpenIDMICO() ([]byte, error) {
	sizes := []int{256, 128, 64, 48, 32, 16}
	frames := make([][]byte, len(sizes))

	for i, sz := range sizes {
		if sz == 256 {
			pngBytes, err := RenderOpenIDMIconPNG(sz)
			if err != nil {
				return nil, err
			}
			frames[i] = pngBytes
		} else {
			frames[i] = encodeBMPDIBFrame(RenderOpenIDMIcon(sz))
		}
	}

	headerSize := 6 + len(sizes)*16
	totalSize := headerSize
	for _, f := range frames {
		totalSize += len(f)
	}

	out := make([]byte, totalSize)
	binary.LittleEndian.PutUint16(out[0:2], 0)                  // Reserved
	binary.LittleEndian.PutUint16(out[2:4], 1)                  // Type = 1 (ICO)
	binary.LittleEndian.PutUint16(out[4:6], uint16(len(sizes))) // Count

	dataOffset := uint32(headerSize)
	for i, sz := range sizes {
		entryOffset := 6 + i*16
		wByte := uint8(sz)
		hByte := uint8(sz)
		if sz >= 256 {
			wByte = 0
			hByte = 0
		}
		out[entryOffset+0] = wByte // bWidth
		out[entryOffset+1] = hByte // bHeight
		out[entryOffset+2] = 0     // bColorCount
		out[entryOffset+3] = 0     // bReserved
		binary.LittleEndian.PutUint16(out[entryOffset+4:entryOffset+6], 1)                       // wPlanes
		binary.LittleEndian.PutUint16(out[entryOffset+6:entryOffset+8], 32)                      // wBitCount
		binary.LittleEndian.PutUint32(out[entryOffset+8:entryOffset+12], uint32(len(frames[i]))) // dwBytesInRes
		binary.LittleEndian.PutUint32(out[entryOffset+12:entryOffset+16], dataOffset)            // dwImageOffset

		copy(out[dataOffset:], frames[i])
		dataOffset += uint32(len(frames[i]))
	}

	return out, nil
}

// EnsureIconAssets grava o arquivo openidm.ico no diretório base e gera os ícones PNG da extensão do navegador.
func EnsureIconAssets(baseDir string) (string, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}

	icoBytes, err := GenerateOpenIDMICO()
	if err != nil {
		return "", err
	}

	icoPath := filepath.Join(baseDir, "openidm.ico")
	if err := os.WriteFile(icoPath, icoBytes, 0o644); err != nil {
		return "", err
	}

	// Se houver pasta browser-extension, garante que os ícones PNG (16, 32, 48, 128) existam nela
	extDir := filepath.Join(baseDir, "browser-extension")
	if info, statErr := os.Stat(extDir); statErr == nil && info.IsDir() {
		iconsDir := filepath.Join(extDir, "icons")
		if mkErr := os.MkdirAll(iconsDir, 0o755); mkErr == nil {
			for _, sz := range []int{16, 32, 48, 128} {
				if pngData, pErr := RenderOpenIDMIconPNG(sz); pErr == nil {
					_ = os.WriteFile(filepath.Join(iconsDir, iconFileName(sz)), pngData, 0o644)
				}
			}
		}
	}

	return icoPath, nil
}

func iconFileName(size int) string {
	switch size {
	case 16:
		return "icon16.png"
	case 32:
		return "icon32.png"
	case 48:
		return "icon48.png"
	default:
		return "icon128.png"
	}
}
