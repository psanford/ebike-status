// Copyright (c) 2018, RetailNext, Inc.
// This material contains trade secrets and confidential information of
// RetailNext, Inc.  Any use, reproduction, disclosure or dissemination
// is strictly prohibited without the explicit written permission
// of RetailNext, Inc.
// All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var addr = flag.String("listen_addr", "127.0.0.1:1234", "Host/Port to listen on")
var local = flag.Bool("local", false, "Run local server")

func main() {
	flag.Parse()
	if *local {
		http.HandleFunc("/", rootHandler)
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
	var (
		resp events.APIGatewayProxyResponse
		b    bytes.Buffer
	)

	err := index(&b)
	if err != nil {
		resp.StatusCode = http.StatusInternalServerError
		return resp, nil
	}

	resp.StatusCode = http.StatusOK
	resp.Body = b.String()
	resp.Headers = map[string]string{
		"Content-Type": "text/html",
	}
	return resp, nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	err := index(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func index(w io.Writer) error {
	rawResp, err := http.Get("https://gbfs.fordgobike.com/gbfs/en/station_status.json")
	if err != nil {
		log.Printf("Get station status error=%s", err)
		return errors.New("Get Status Failed")
	}

	dec := json.NewDecoder(rawResp.Body)
	var resp StationStatusResponse

	if err := dec.Decode(&resp); err != nil {
		log.Printf("Decode station status error=%s", err)
		return errors.New("Get Status Failed")
	}

	stations := make(map[string]StationStatus)
	for _, sta := range resp.Data.Stations {
		stations[sta.ID] = sta
	}

	d := TmplData{
		Regions: populateCounts(stations),
	}

	if err := tmpl.Execute(w, d); err != nil {
		log.Printf("Execute template error=%s", err)
		return errors.New("Template Error")
	}

	return nil
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
    </style>
    <link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.1/css/bootstrap.min.css" integrity="sha384-WskhaSGFgHYWDcbwN70/dfYBj47jz9qbsMId/iRN3ewGhXQFZCSftd1LZCfmhktB" crossorigin="anonymous">
	</head>
	<body>
    <main class="py-md-3">
    {{range .Regions}}
      <div class="container">
	    <h2>{{.Name}}:</h2>
	    <table class="table">
	    {{range .Stations}}
	      <tr class="{{.CSSClass}}"><td>{{.ID}}</td><td>{{.Name}}</td><td>{{range .Bars}}<div class=bar></div>{{end}}</td></tr>
	    {{end}}
	    </table>
	    </div>
	  {{end}}
    </main>
	</body>
</html>`

var tmpl = template.Must(template.New("html").Parse(tmplText))

type TmplData struct {
	Regions []Region
}
