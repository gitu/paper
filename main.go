package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dkit"
	"image"
	"image/color"

	"github.com/gitu/paper/fonts"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d"
	"golang.org/x/image/bmp"
	"math/rand"
	"os"
)

func withLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("ts=%s url=%s remoteAddr=%s", time.Now(), r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	}
}

//go:generate go-bindata -pkg fonts -prefix "fonts/" -o fonts/bindata.go -ignore bindata.go fonts/
func getFont(typ string) *truetype.Font {
	bytes, e := fonts.Asset("Roboto-" + typ + ".ttf")
	if e != nil {
		log.Println("error", e)
	}
	font, e := freetype.ParseFont(bytes)
	if e != nil {
		log.Println("error", e)
	}
	return font
}

var red = color.RGBA{0xff, 0x00, 0x00, 0xff}
var black = color.RGBA{0x00, 0x00, 0x00, 0xff}
var white = color.RGBA{0xff, 0xff, 0xff, 0xff}

var width, height = 640, 384
var fwidth, fheight = float64(width), float64(height)

type BlockInfo struct {
	Time    string
	Blocked [24]bool
}

type Schedule struct {
	Name       string
	Blocked    bool
	BlockInfos [4]BlockInfo
}

func serveClock(w http.ResponseWriter, _ *http.Request) {
	rand.Seed(int64(time.Now().Minute()))
	schedule := Schedule{
		Blocked: rand.Float32() > 0.5,
		Name:    "Meeting Room A",
	}
	log.Println("Blocked", schedule.Blocked)

	blocked := schedule.Blocked
	for i := 0; i < len(schedule.BlockInfos); i++ {
		for j := 0; j < len(schedule.BlockInfos[i].Blocked); j++ {
			if rand.Float32() > 0.9 {
				blocked = !blocked
			}
			schedule.BlockInfos[i].Blocked[j] = blocked
		}
		schedule.BlockInfos[i].Time = fmt.Sprintf("%02d:00", time.Now().Hour()+i)
	}

	drawClock(schedule, w)
}

func drawClock(schedule Schedule, w http.ResponseWriter) {
	dest := image.NewRGBA(image.Rect(0, 0, width, height))
	gc := draw2dimg.NewGraphicContext(dest)
	draw2dkit.Rectangle(gc, 0, 0, fwidth, fheight)
	gc.SetFillColor(white)
	gc.FillStroke()
	// Set some properties
	gc.SetFillColor(black)
	gc.SetStrokeColor(black)
	gc.SetLineWidth(5)
	gc.FontCache.Store(draw2d.FontData{Name: "roboto"}, getFont("Regular"))
	gc.FontCache.Store(draw2d.FontData{Name: "roboto-bold"}, getFont("Bold"))
	gc.SetFontData(draw2d.FontData{Name: "roboto-bold"})
	// Clock
	gc.SetFontSize(35)
	gc.FillStringAt(schedule.Name, 85, 70)
	drawQuarters(gc, schedule)

	// Save to file
	bmp.Encode(w, dest)
}

func drawQuarters(gc *draw2dimg.GraphicContext, schedule Schedule) {

	lines := len(schedule.BlockInfos)
	startHeight, heightLine := 100.0, 50.0
	heightEnd := startHeight + heightLine*float64(lines)
	border := 85.0
	widthEnd := fwidth - border
	middleLine := 150.0

	if schedule.Blocked {
		gc.SetStrokeColor(red)
		gc.SetFillColor(red)
		draw2dkit.Rectangle(gc, border, heightEnd+25, widthEnd+2, heightEnd+50)
		gc.FillStroke()

	}
	gc.SetStrokeColor(black)
	gc.SetFillColor(black)

	gc.SetLineWidth(2)
	gc.MoveTo(border, startHeight)
	gc.LineTo(border, heightEnd)

	gc.MoveTo(middleLine-2, startHeight)
	gc.LineTo(middleLine-2, heightEnd)

	gc.MoveTo(widthEnd+2, startHeight)
	gc.LineTo(widthEnd+2, heightEnd)

	for i := 0; i <= lines; i++ {
		gc.MoveTo(border, startHeight+heightLine*float64(i))
		gc.LineTo(widthEnd+2, startHeight+heightLine*float64(i))
	}
	gc.Stroke()

	gc.SetFontData(draw2d.FontData{Name: "roboto-bold"})
	for i := 1; i <= lines; i++ {
		gc.SetFontSize(16)
		gc.FillStringAt(schedule.BlockInfos[i-1].Time, border+5, startHeight+heightLine*float64(i)-17)
	}

	for i := 0; i < lines; i++ {
		cols := len(schedule.BlockInfos[i].Blocked)
		colWidth := (widthEnd - middleLine) / float64(cols)
		for j := 0; j < cols; j++ {
			if schedule.BlockInfos[i].Blocked[j] {
				draw2dkit.RoundedRectangle(gc,
					middleLine+colWidth*float64(j)+3, startHeight+heightLine*float64(i)+3,
					middleLine+colWidth*float64(j+1)-3, startHeight+heightLine*float64(i+1)-3,
					5, 5)
			}
		}
	}
	gc.FillStroke()
}

func main() {
	http.HandleFunc("/clock", withLogging(serveClock))

	addr := ""
	port := os.Getenv("PORT")

	if port == "" {
		log.Println("$PORT should be set")
		addr = "127.0.0.1"
		port = "8080"
	}

	log.Fatal(http.ListenAndServe(addr+":"+port, nil))
}
