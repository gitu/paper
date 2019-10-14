package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os/signal"
	"time"

	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dkit"
	"image"
	"image/color"

	"github.com/PuloV/ics-golang"
	"github.com/gitu/paper/fonts"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d"
	"golang.org/x/image/bmp"
	"math/rand"
	"os"
	"regexp"
	"strings"
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
	Blocked [12]bool
}

type Schedule struct {
	Name       string
	Date       string
	Blocked    bool
	BlockInfos [4]BlockInfo
}

func init() {
	ics.RepeatRuleApply = true
	ics.MaxRepeats = 100
}

func buildSchedule(url, timezone, overrideTimezone, name string) (schedule Schedule, err error) {
	schedule = Schedule{}

	response, err := http.Get(url)
	if err != nil {
		log.Println("err", err)
		return schedule, err
	}
	// close the response body
	defer response.Body.Close()

	icsBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Println("err", err)
		return schedule, err
	}

	calendar, _, err := ics.ParseICalContent(string(icsBytes), url)
	if err != nil {
		log.Println("err", err)
		return schedule, err
	}
	schedule.Name = name

	// print calendar info
	//log.Println(calendar.GetName(), calendar.GetDesc())

	ttz, err := time.LoadLocation(timezone)
	if err != nil {
		return schedule, err
	}

	otz, err := time.LoadLocation(overrideTimezone)
	if err != nil {
		otz = ttz
	}

	//  get the events for the New Years Eve
	now := time.Now().In(ttz)

	startBlocker := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, otz)
	endBlocker := startBlocker.Add(time.Duration(len(schedule.BlockInfos)) * time.Hour).Add(time.Hour)
	nowForBlock := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, otz)

	//log.Printf("    %s %s \n", startBlocker, endBlocker)

	for i := 0; i < len(schedule.BlockInfos); i++ {
		schedule.BlockInfos[i].Time = fmt.Sprintf("%02d:00", time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, ttz).Add(time.Duration(i)*time.Hour).Hour())
	}
	schedule.Date = now.Format("02.01.2006")

	for _, event := range calendar.GetEvents() {
		if event.GetStart().Before(nowForBlock) && event.GetEnd().After(nowForBlock) {
			schedule.Blocked = true
			//log.Printf("blocked - %s %s \n", event.GetStart(), event.GetEnd())
		}
		blocksPerHour := len(schedule.BlockInfos[0].Blocked)
		totalBlocks := blocksPerHour * len(schedule.BlockInfos)

		if event.GetStart().Before(endBlocker) && event.GetEnd().After(startBlocker) {
			startBlock := (hours(event.GetStart())-hours(startBlocker))*blocksPerHour + (event.GetStart().Minute()*blocksPerHour)/60
			endBlock := (hours(event.GetEnd())-hours(startBlocker))*blocksPerHour + (event.GetEnd().Minute()*blocksPerHour)/60

			if startBlock < 0 {
				startBlock = 0
			}
			for b := startBlock; b < totalBlocks && b < endBlock; b++ {
				schedule.BlockInfos[b/blocksPerHour].Blocked[b%blocksPerHour] = true
			}

			//log.Printf("%s %s  %d - %d \n", event.GetStart(), event.GetEnd(), startBlock, endBlock)
		}

	}

	return schedule, nil
}
func hours(now time.Time) int {
	return int(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC).Unix() / 3600)
}

func serveClock(w http.ResponseWriter, r *http.Request) {
	var schedule Schedule
	var err error
	display := r.URL.Query().Get("display")
	if display != "" {
		sanDisplay := sanitize(display)
		// log.Println("display", display, "sanitized", sanDisplay)

		name := os.Getenv("DISPLAY_" + sanDisplay + "_NAME")
		url := os.Getenv("DISPLAY_" + sanDisplay + "_URL")
		tz := os.Getenv("DISPLAY_" + sanDisplay + "_TZ")
		otz := os.Getenv("DISPLAY_" + sanDisplay + "_OTZ")

		if url != "" && tz != "" {
			schedule, err = buildSchedule(url, tz, otz, name)
			if err != nil {
				log.Println(err)
				w.WriteHeader(500)
				return
			}
		} else {
			log.Println("not found", sanDisplay)
		}
	}
	if schedule.Name == "" {
		schedule = randomSchedule(int64(time.Now().Minute()))
	}
	err = drawClock(schedule, w)

	if err != nil {
		log.Println(err)
	}
}

var regular = getFont("Regular")
var bold = getFont("Bold")

var notWhitelist = regexp.MustCompile(`[^0-9A-Z]`)

func sanitize(s string) string {
	return notWhitelist.ReplaceAllString(strings.ToUpper(s), "")
}

func randomSchedule(seed int64) Schedule {
	rand.Seed(seed)
	schedule := Schedule{
		Blocked: rand.Float32() > 0.5,
		Name:    "Random Room",
		Date:    time.Now().Format("2006-01-02"),
	}
	blocked := schedule.Blocked
	for i := 0; i < len(schedule.BlockInfos); i++ {
		for j := 0; j < len(schedule.BlockInfos[i].Blocked); j++ {
			if rand.Float32() > 0.95 {
				blocked = !blocked
			}
			schedule.BlockInfos[i].Blocked[j] = blocked
		}
		schedule.BlockInfos[i].Time = fmt.Sprintf("%02d:00", time.Now().Hour()+i)
	}
	return schedule
}

func drawClock(schedule Schedule, w io.Writer) error {
	dest := image.NewRGBA(image.Rect(0, 0, width, height))
	gc := draw2dimg.NewGraphicContext(dest)
	draw2dkit.Rectangle(gc, 0, 0, fwidth, fheight)
	gc.SetFillColor(white)
	gc.FillStroke()
	// Set some properties
	gc.SetFillColor(black)
	gc.SetStrokeColor(black)
	gc.SetLineWidth(5)
	gc.FontCache.Store(draw2d.FontData{Name: "roboto"}, regular)
	gc.FontCache.Store(draw2d.FontData{Name: "roboto-bold"}, bold)
	gc.SetFontData(draw2d.FontData{Name: "roboto-bold"})
	// Clock
	gc.SetFontSize(30)
	gc.FillStringAt(schedule.Name, 85, 70)
	gc.SetFontSize(20)
	gc.FillStringAt(schedule.Date, 425, 60)
	drawQuarters(gc, schedule)

	// Save to file
	return bmp.Encode(w, dest)
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
	gc.SetFontSize(16)
	for i := 1; i <= lines; i++ {
		gc.FillStringAt(schedule.BlockInfos[i-1].Time, border+5, startHeight+heightLine*float64(i)-17)
	}

	gc.SetFontData(draw2d.FontData{Name: "roboto-bold"})
	gc.SetFontSize(13)
	for i := 0; i < 4; i++ {
		colWidth := (widthEnd - middleLine) / float64(4)
		gc.FillStringAt(fmt.Sprintf(":%02d", 15*i), middleLine+colWidth*float64(i), startHeight-4)

	}

	for i := 0; i < lines; i++ {
		cols := len(schedule.BlockInfos[i].Blocked)
		colWidth := (widthEnd - middleLine) / float64(cols)
		for j := 0; j < cols; j++ {
			if schedule.BlockInfos[i].Blocked[j] {
				draw2dkit.RoundedRectangle(gc,
					middleLine+colWidth*float64(j)+4, startHeight+heightLine*float64(i)+4,
					middleLine+colWidth*float64(j+1)-4, startHeight+heightLine*float64(i+1)-4,
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

	server := &http.Server{Addr: addr + ":" + port, Handler: nil}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	go func() {
		log.Println("Listening on " + addr + ":" + port)
		if err := server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	<-stop
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	server.Shutdown(ctx)
	log.Println("Server gracefully stopped")
}
