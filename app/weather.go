package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"strings"
	"time"

	"deskui/source/hass"
	"deskui/widget"
)

// Nord colors for the weather icons.
var (
	iconSun   = color.RGBA{0xeb, 0xcb, 0x8b, 0xff} // nord13
	iconCloud = color.RGBA{0xd8, 0xde, 0xe9, 0xff} // nord4
	iconDark  = color.RGBA{0x81, 0x8c, 0xa3, 0xff} // rain/storm clouds
	iconRain  = color.RGBA{0x88, 0xc0, 0xd0, 0xff} // nord8
	iconSnow  = color.RGBA{0xec, 0xef, 0xf4, 0xff} // nord6
	iconBolt  = color.RGBA{0xeb, 0xcb, 0x8b, 0xff} // nord13
)

// weatherView shows a condition icon, the current temperature and today's
// high/low. It draws nothing until the first state arrives.
type weatherView struct {
	widget.Base
	th        *widget.Theme
	condition string // Home Assistant condition, e.g. "partlycloudy"
	temp      string // "12°"
	detail    string // "14°/6°", or the condition's name without a forecast
}

func newWeatherView(th *widget.Theme) *weatherView { return &weatherView{th: th} }

func (w *weatherView) set(condition, temp, detail string) {
	if condition != w.condition || temp != w.temp || detail != w.detail {
		w.condition, w.temp, w.detail = condition, temp, detail
		w.Invalidate()
	}
}

func (w *weatherView) Draw(c *widget.Canvas) {
	if w.condition == "" && w.temp == "" {
		return
	}
	r := w.Bounds()
	const icon = 48
	ir := image.Rect(r.Min.X, r.Min.Y+(r.Dy()-icon)/2, r.Min.X+icon, r.Min.Y+(r.Dy()+icon)/2)
	c.WithClip(ir, func() { drawWeatherIcon(c, w.condition, ir, w.th.Background) })

	tr := r
	tr.Min.X = ir.Max.X + 6
	mid := r.Min.Y + r.Dy()/2
	c.Text(w.temp, w.th.Medium, w.th.Text, image.Rect(tr.Min.X, r.Min.Y, tr.Max.X, mid+2), 0, 1)
	c.Text(w.detail, w.th.Small, w.th.TextDim, image.Rect(tr.Min.X, mid+2, tr.Max.X, r.Max.Y), 0, 0)
}

// drawWeatherIcon draws a Home Assistant weather condition into r (square).
// bg is the color behind the icon, used to cut the moon's crescent.
func drawWeatherIcon(c *widget.Canvas, cond string, r image.Rectangle, bg color.Color) {
	s := float64(r.Dx())
	x0, y0 := float64(r.Min.X), float64(r.Min.Y)
	// Coordinates below are in a 0..1 box.
	P := func(x, y float64) (float64, float64) { return x0 + x*s, y0 + y*s }
	L := func(v float64) float64 { return v * s }

	sun := func(cx, cy, rad float64) {
		x, y := P(cx, cy)
		c.SetColor(iconSun)
		c.DrawCircle(x, y, L(rad))
		c.Fill()
		c.SetLineWidth(L(0.05))
		c.SetLineCapRound()
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			c.DrawLine(x+math.Cos(a)*L(rad*1.45), y+math.Sin(a)*L(rad*1.45),
				x+math.Cos(a)*L(rad*1.9), y+math.Sin(a)*L(rad*1.9))
		}
		c.Stroke()
	}
	moon := func() {
		x, y := P(0.5, 0.5)
		drawMoon(c, x, y, L(0.32), iconSun, bg)
	}
	// cloud draws a cloud whose bottom edge sits at y (0..1).
	cloud := func(col color.Color, dx, y float64) {
		c.SetColor(col)
		for _, b := range [][3]float64{{0.30, -0.13, 0.15}, {0.52, -0.22, 0.20}, {0.72, -0.12, 0.14}} {
			x, cy := P(b[0]+dx, y+b[1])
			c.DrawCircle(x, cy, L(b[2]))
		}
		c.Fill()
		// Flat bottom joining the circles.
		xa, ya := P(0.15+dx, y-0.15)
		c.DrawRoundedRectangle(xa, ya, L(0.71), L(0.15), L(0.075))
		c.Fill()
	}
	drops := func(n int) {
		c.SetColor(iconRain)
		c.SetLineWidth(L(0.055))
		c.SetLineCapRound()
		for i := 0; i < n; i++ {
			x, y := P(0.30+float64(i)*0.4/float64(max(n-1, 1)), 0.70)
			c.DrawLine(x, y, x-L(0.06), y+L(0.17))
		}
		c.Stroke()
	}
	flakes := func(xs ...float64) {
		c.SetColor(iconSnow)
		for i, fx := range xs {
			x, y := P(fx, 0.76+float64(i%2)*0.1)
			c.DrawCircle(x, y, L(0.045))
		}
		c.Fill()
	}
	bolt := func() {
		c.SetColor(iconBolt)
		pts := [][2]float64{{0.52, 0.62}, {0.40, 0.82}, {0.50, 0.82}, {0.44, 0.98}, {0.62, 0.74}, {0.52, 0.74}, {0.58, 0.62}}
		for i, p := range pts {
			x, y := P(p[0], p[1])
			if i == 0 {
				c.MoveTo(x, y)
			} else {
				c.LineTo(x, y)
			}
		}
		c.ClosePath()
		c.Fill()
	}
	lines := func(col color.Color, ys []float64, xs [][2]float64) {
		c.SetColor(col)
		c.SetLineWidth(L(0.06))
		c.SetLineCapRound()
		for i, yy := range ys {
			xa, y := P(xs[i][0], yy)
			xb, _ := P(xs[i][1], yy)
			c.DrawLine(xa, y, xb, y)
		}
		c.Stroke()
	}

	switch cond {
	case "sunny":
		sun(0.5, 0.5, 0.2)
	case "clear-night":
		moon()
	case "partlycloudy":
		sun(0.64, 0.36, 0.14)
		cloud(iconCloud, -0.06, 0.82)
	case "cloudy":
		cloud(iconCloud, 0, 0.72)
	case "fog":
		lines(iconCloud, []float64{0.34, 0.50, 0.66}, [][2]float64{{0.18, 0.72}, {0.28, 0.82}, {0.18, 0.66}})
	case "windy", "windy-variant":
		lines(iconCloud, []float64{0.36, 0.52, 0.68}, [][2]float64{{0.14, 0.70}, {0.14, 0.86}, {0.14, 0.58}})
	case "rainy":
		cloud(iconDark, 0, 0.62)
		drops(3)
	case "pouring":
		cloud(iconDark, 0, 0.62)
		drops(4)
	case "snowy":
		cloud(iconCloud, 0, 0.62)
		flakes(0.32, 0.50, 0.68)
	case "snowy-rainy", "hail":
		cloud(iconDark, 0, 0.62)
		drops(2)
		flakes(0.40, 0.66)
	case "lightning":
		cloud(iconDark, 0, 0.62)
		bolt()
	case "lightning-rainy":
		cloud(iconDark, 0, 0.62)
		drops(2)
		bolt()
	default: // "exceptional", unknown
		cloud(iconDark, 0, 0.72)
	}
}

// drawMoon draws a crescent of radius r centered at (x, y), cut out with bg.
func drawMoon(c *widget.Canvas, x, y, r float64, fg, bg color.Color) {
	c.SetColor(fg)
	c.DrawCircle(x, y, r)
	c.Fill()
	c.SetColor(bg)
	c.DrawCircle(x+r*0.53, y-r*0.375, r*0.84)
	c.Fill()
}

var conditionNames = map[string]string{
	"clear-night": "Clear", "cloudy": "Cloudy", "exceptional": "Exceptional",
	"fog": "Fog", "hail": "Hail", "lightning": "Thunder", "lightning-rainy": "Thunder",
	"partlycloudy": "Partly cloudy", "pouring": "Pouring", "rainy": "Rain",
	"snowy": "Snow", "snowy-rainy": "Sleet", "sunny": "Sunny",
	"windy": "Windy", "windy-variant": "Windy",
}

// forecastDay is one entry of a weather.get_forecasts response.
type forecastDay struct {
	Datetime    time.Time `json:"datetime"`
	Temperature *float64  `json:"temperature"`
	TempLow     *float64  `json:"templow"`
}

// todayHighLow extracts today's high and low from a weather.get_forecasts
// response for entity. Daily forecasts give them directly; hourly ones are
// reduced over the rest of today.
func todayHighLow(resp json.RawMessage, entity string, now time.Time) (hi, lo float64, ok bool) {
	var r map[string]struct {
		Forecast []forecastDay `json:"forecast"`
	}
	if json.Unmarshal(resp, &r) != nil {
		return 0, 0, false
	}
	fs := r[entity].Forecast
	for _, f := range fs {
		if f.TempLow != nil && f.Temperature != nil && sameDay(f.Datetime.In(now.Location()), now) {
			return *f.Temperature, *f.TempLow, true
		}
	}
	hi, lo = math.Inf(-1), math.Inf(1)
	for _, f := range fs {
		if f.Temperature != nil && sameDay(f.Datetime.In(now.Location()), now) {
			hi, lo, ok = max(hi, *f.Temperature), min(lo, *f.Temperature), true
		}
	}
	return hi, lo, ok
}

func fmtTemp(v float64) string {
	s := fmt.Sprintf("%.0f°", v)
	if s == "-0°" {
		s = "0°"
	}
	return s
}

// setWeatherState updates the view from the weather entity's state.
func (a *app) setWeatherState(s hass.State) {
	temp := ""
	if t, ok := s.Attributes["temperature"].(float64); ok {
		temp = fmtTemp(t)
	}
	if s.State == "unavailable" || s.State == "unknown" {
		a.weather.set("", "", "")
		return
	}
	a.weatherCond = s.State
	a.weather.set(s.State, temp, a.weatherDetail())
}

func (a *app) setForecast(hi, lo float64, ok bool) {
	a.hiLo = ""
	if ok {
		a.hiLo = fmtTemp(hi) + "/" + fmtTemp(lo)
	}
	a.weather.set(a.weather.condition, a.weather.temp, a.weatherDetail())
}

func (a *app) weatherDetail() string {
	if a.hiLo != "" {
		return a.hiLo
	}
	if n, ok := conditionNames[a.weatherCond]; ok {
		return n
	}
	return strings.ReplaceAll(a.weatherCond, "-", " ")
}

// pollForecast fetches today's forecast on every (re)connect and every 30
// minutes. Runs on its own goroutine.
func (a *app) pollForecast(entity string, connected <-chan struct{}) {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-connected:
		case <-t.C:
		}
		hi, lo, ok, err := a.fetchForecast(entity)
		if err != nil {
			fmt.Fprintf(os.Stderr, "home assistant: forecast %s: %v\n", entity, err)
		}
		a.post(func() { a.setForecast(hi, lo, ok) })
	}
}

func (a *app) fetchForecast(entity string) (hi, lo float64, ok bool, err error) {
	for _, typ := range []string{"daily", "hourly"} {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		var resp json.RawMessage
		resp, err = a.ha.CallServiceResponse(ctx, "weather", "get_forecasts", entity, map[string]any{"type": typ})
		cancel()
		if err != nil {
			continue // e.g. the integration doesn't offer this forecast type
		}
		if hi, lo, ok = todayHighLow(resp, entity, time.Now()); ok {
			return hi, lo, true, nil
		}
	}
	return 0, 0, false, err
}
