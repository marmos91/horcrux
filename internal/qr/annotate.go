package qr

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const annotationHeight = 40

// ShardLabel holds human-readable metadata for annotating a QR code image.
type ShardLabel struct {
	Filename   string
	ShardIndex int
	ShardTotal int
	ShardType  string // "data" or "parity"
}

// Summary returns a human-readable label like "Shard 1/5 (data)".
func (l ShardLabel) Summary() string {
	return fmt.Sprintf("Shard %d/%d (%s)", l.ShardIndex+1, l.ShardTotal, l.ShardType)
}

// Annotate composites a QR code image with a text annotation area below it.
// Line 1: shard filename. Line 2: "Shard X/Y (type)".
func Annotate(qrImg image.Image, label ShardLabel) image.Image {
	bounds := qrImg.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, w, h+annotationHeight))
	draw.Draw(dst, image.Rect(0, 0, w, h), qrImg, bounds.Min, draw.Src)
	draw.Draw(dst, image.Rect(0, h, w, h+annotationHeight), image.NewUniform(color.White), image.Point{}, draw.Src)

	face := basicfont.Face7x13
	drawString(dst, face, 8, h+14, label.Filename, color.Black)
	drawString(dst, face, 8, h+30, label.Summary(), color.Black)

	return dst
}

func drawString(img *image.RGBA, face font.Face, x, y int, s string, col color.Color) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	d.DrawString(s)
}
