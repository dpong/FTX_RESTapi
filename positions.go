package api

import (
	"net/http"
)

type PositionResponse struct {
	Success bool         `json:"success"`
	Result  []*Positions `json:"result"`
}

type Positions struct {
	Cost                         float64 `json:"cost"`
	EntryPrice                   float64 `json:"entryPrice"`
	Future                       string  `json:"future"`
	InitialMarginRequirement     float64 `json:"initialMarginRequirement"`
	LongOrderSize                float64 `json:"longOrderSize"`
	MaintenanceMarginRequirement float64 `json:"maintenanceMarginRequirement"`
	NetSize                      float64 `json:"netSize"`
	OpenSize                     float64 `json:"openSize"`
	RealizedPnl                  float64 `json:"realizedPnl"`
	ShortOrderSize               float64 `json:"shortOrderSize"`
	Side                         string  `json:"side"`
	Size                         float64 `json:"size"`
	UnrealizedPnl                float64 `json:"unrealizedPnl"`
	AvgPrice                     float64 `json:"recentAverageOpenPrice, omitempty"`
	BreakEvenPrice               float64 `json:"recentBreakEvenPrice, omitempty"`
}

func (p *Client) Positions() (positions *PositionResponse, err error) {
	params := make(map[string]string)
	params["showAvgPrice"] = "true"
	res, err := p.sendRequest(
		http.MethodGet,
		"/positions",
		nil, &params)
	if err != nil {
		return nil, err
	}
	// in Close()
	err = decode(res, &positions)
	if err != nil {
		return nil, err
	}
	return positions, nil
}
