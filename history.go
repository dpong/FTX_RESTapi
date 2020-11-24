package api

import (
	"fmt"
	"net/http"
	"time"
)

type HistoryResponse struct {
	Success bool          `json:"success"`
	Result  []HistoryData `json:"result"`
}

type HistoryData struct {
	Close  float64   `json:"close"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Open   float64   `json:"open"`
	Time   time.Time `json:"startTime"`
	Volume float64   `json:"volume"`
}

func (p *Client) HistoryData(ticker string, start, end time.Time, resolution int) (historys *HistoryResponse) {
	params := make(map[string]string)
	if start.IsZero() == true && end.IsZero() == true {
		params["limit"] = fmt.Sprintf("%v", 5000)
	} else {
		params["start_time"] = fmt.Sprintf("%v", start.Unix())
		params["end_time"] = fmt.Sprintf("%v", end.Unix())
	}
	params["resolution"] = fmt.Sprintf("%v", resolution)
	res, err := p.sendRequest(
		http.MethodGet,
		fmt.Sprintf(
			"/markets/%s/candles",
			ticker),
		nil, &params)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &historys)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return historys
}
