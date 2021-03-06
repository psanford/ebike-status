package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var addr = flag.String("listen_addr", "127.0.0.1:1234", "Host/Port to listen on")
var local = flag.Bool("local", false, "Run local server")

var stationURL = "https://gbfs.baywheels.com/gbfs/es/station_status.json"

func main() {
	flag.Parse()
	if *local {
		http.HandleFunc("/", index)
		panic(http.ListenAndServe(*addr, nil))
	} else {
		lambda.Start(AWSHandler)
	}
}

type StationInfo struct {
	ID       string
	Name     string
	Count    int
	CSSClass color
}

func (s *StationInfo) Bars() []struct{} {
	return make([]struct{}, s.Count)
}

type Region struct {
	Name     string
	Stations []StationInfo
}

var Regions = []Region{
	{
		Name: "Embarcadero",
		Stations: []StationInfo{
			{
				ID:   "22",
				Name: "Howard St at Beale St",
			},
			{
				ID:   "17",
				Name: "Beale St at Market St",
			},
			{
				ID:   "20",
				Name: "Market St at Bush St",
			},
			{
				ID:   "27",
				Name: "Beale St at Harrison St",
			},
		},
	},
	{
		Name: "Mission",
		Stations: []StationInfo{
			{
				ID:   "139",
				Name: "25th St at Harrison St",
			},
			{
				ID:   "129",
				Name: "Harrison St at 20th St",
			},
			{
				ID:   "125",
				Name: "20th St at Bryant St",
			},
			{
				ID:   "124",
				Name: "19th St at Florida St",
			},
		},
	},
	{
		Name: "Potrero",
		Stations: []StationInfo{
			{
				ID:   "130",
				Name: "22nd St Caltrain Station",
			},
			{
				ID:   "126",
				Name: "Esprit Park",
			},
		},
	},
}

func AWSHandler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	w := newRespsonseWriter()
	r := newRequest(req)

	index(w, r)

	return w.Response(), nil
}

func newRespsonseWriter() *ResponseWriter {
	header := make(http.Header)
	header.Set("Content-Type", "text/html")

	return &ResponseWriter{
		header:     header,
		statusCode: http.StatusOK,
	}
}

func newRequest(req events.APIGatewayProxyRequest) *http.Request {
	var params url.Values
	for k, v := range req.QueryStringParameters {
		params.Set(k, v)
	}

	u := url.URL{
		Host:     req.Headers["Host"],
		Scheme:   req.Headers["X-Forwarded-Proto"],
		Path:     req.Path,
		RawQuery: params.Encode(),
	}

	httpReq, err := http.NewRequest(req.HTTPMethod, u.String(), bytes.NewReader([]byte(req.Body)))
	if err != nil {
		panic(err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	httpReq.RemoteAddr = req.RequestContext.Identity.SourceIP
	return httpReq
}

type ResponseWriter struct {
	header     http.Header
	b          bytes.Buffer
	statusCode int
}

func (w *ResponseWriter) Header() http.Header {
	return w.header
}

func (w *ResponseWriter) Write(p []byte) (int, error) {
	return w.b.Write(p)
}

func (w *ResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *ResponseWriter) Response() events.APIGatewayProxyResponse {
	headers := make(map[string]string)
	for k, vals := range w.header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}

	return events.APIGatewayProxyResponse{
		StatusCode: w.statusCode,
		Headers:    headers,
		Body:       w.b.String(),
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	rawResp, err := http.Get(stationURL)
	if err != nil {
		log.Printf("Get station status error=%s", err)
		http.Error(w, "Get Status Failed", http.StatusInternalServerError)
		return
	}

	dec := json.NewDecoder(rawResp.Body)
	var resp StationStatusResponse

	if err := dec.Decode(&resp); err != nil {
		log.Printf("Decode station status error=%s", err)
		http.Error(w, "Get Status Failed", http.StatusInternalServerError)
		return
	}

	var (
		classicCount int
		ebikeCount   int
		stations     = make(map[string]StationStatus)
	)
	for _, sta := range resp.Data.Stations {
		classicCount += sta.NumBikesAvailable
		ebikeCount += sta.NumEbikesAvailable
		stations[sta.ID] = sta
	}

	regions := populateCounts(stations)

	ua := strings.ToLower(r.UserAgent())

	if r.Header.Get("Accept") == "text/plain" || strings.HasPrefix(ua, "curl/") {
		w.Header().Set("Content-Type", "text/plain")
		format := "%12.12s %25.25s %d\n"
		for _, r := range regions {
			for _, s := range r.Stations {
				fmt.Fprintf(w, format, r.Name, s.Name, s.Count)
			}
		}
		return
	}

	d := TmplData{
		EbikeCount:   ebikeCount,
		ClassicCount: classicCount,
		Regions:      regions,
	}

	if err := tmpl.Execute(w, d); err != nil {
		log.Printf("Execute template error=%s", err)
		http.Error(w, "Template Error", http.StatusInternalServerError)
		return
	}
}

func populateCounts(stations map[string]StationStatus) []Region {
	outRegions := make([]Region, 0, len(Regions))
	for _, r := range Regions {
		out := r
		out.Stations = make([]StationInfo, 0, len(r.Stations))
		for _, inf := range r.Stations {
			status, ok := stations[inf.ID]

			if !ok {
				inf.CSSClass = red
			}
			inf.Count = status.NumEbikesAvailable
			out.Stations = append(out.Stations, inf)
		}
		outRegions = append(outRegions, out)
	}
	return outRegions
}

type color string

const (
	green  color = "green"
	yellow color = "yellow"
	red    color = "red"
)

type StationStatusResponse struct {
	Data struct {
		Stations []StationStatus `json:"stations"`
	} `json:"data"`
	LastUpdated float64 `json:"last_updated"`
	Ttl         float64 `json:"ttl"`
}

type StationStatus struct {
	EightdActiveStationServices []struct {
		ID string `json:"id"`
	} `json:"eightd_active_station_services"`
	EightdHasAvailableKeys bool   `json:"eightd_has_available_keys"`
	IsInstalled            int    `json:"is_installed"`
	IsRenting              int    `json:"is_renting"`
	IsReturning            int    `json:"is_returning"`
	LastReported           int    `json:"last_reported"`
	NumBikesAvailable      int    `json:"num_bikes_available"`
	NumBikesDisabled       int    `json:"num_bikes_disabled"`
	NumDocksAvailable      int    `json:"num_docks_available"`
	NumDocksDisabled       int    `json:"num_docks_disabled"`
	NumEbikesAvailable     int    `json:"num_ebikes_available"`
	ID                     string `json:"station_id"`
}

const tmplText = `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
		<title>Ebike status</title>
    <style>
      .green { background-color: green; }
      .yellow { background-color: yellow; }
      .red { background-color: red; }
      .bar {
        display: inline-block;
        width: .25em;
        height: 1em;
        background-color: green;
        margin-right: .25em;
        float: right;
      }
      .row-pad {
        padding-top: 12px;
        padding-bottom: 12px;
      }
    </style>
    <link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.1/css/bootstrap.min.css" integrity="sha384-WskhaSGFgHYWDcbwN70/dfYBj47jz9qbsMId/iRN3ewGhXQFZCSftd1LZCfmhktB" crossorigin="anonymous">
	</head>
	<body>
    <main class="py-md-3">
    {{range .Regions}}
      <div class="container">
	    <h2>{{.Name}}:</h2>
	    <div class="table">
	    {{range .Stations}}
	      <div class="{{.CSSClass}} row row-pad border-top"><div class="col">{{.ID}}</div><div class="col">{{.Name}}</div><div class="col">{{range .Bars}}<div class=bar></div>{{end}}</div></div>
	    {{end}}
	    </div>
	    </div>
	  {{end}}
      <br><br>
      <div class="container">
	    <h4>SystemInfo:</h4>
      <div class=table>
      <div class="row row-pad border-top"><div class=col>Available E-bikes:</div><div class=col>{{.EbikeCount}}</div><div class="col"></div></div>
      <div class="row row-pad border-top"><div class=col>Available Classic Bikes:</div><div class=col>{{.ClassicCount}}</div><div class="col"></div></div>
      </div>
      </div>
    </main>
	</body>
</html>`

var tmpl = template.Must(template.New("html").Parse(tmplText))

type TmplData struct {
	Regions      []Region
	EbikeCount   int
	ClassicCount int
}
