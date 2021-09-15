package api

import (
	"net/http"
	"time"
)

type FundingResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Future string    `json:"future"`
		Rate   float64   `json:"rate"`
		Time   time.Time `json:"time"`
	} `json:"result"`
}

type FundingPaymentResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Future  string    `json:"future"`
		Payment float64   `json:"payment"`
		Rate    float64   `json:"rate"`
		Time    time.Time `json:"time"`
	} `json:"result"`
}

func (p *Client) Fundings(symbol, start, end string) (futures *FundingResponse, err error) {
	params := make(map[string]string)
	if symbol != "" {
		params["future"] = symbol
	}
	if start != "" {
		params["start_time"] = start
	}
	if end != "" {
		params["end_time"] = end
	}
	res, err := p.sendRequest(http.MethodGet, "/funding_rates", nil, &params)
	if err != nil {
		return nil, err
	}
	// in Close()
	err = decode(res, &futures)
	if err != nil {
		return nil, err
	}
	return futures, nil
}

func (p *Client) FundingsPayment(symbol string) (futures *FundingPaymentResponse, err error) {
	params := make(map[string]string)
	if symbol != "" {
		params["future"] = symbol
	}
	res, err := p.sendRequest(http.MethodGet, "/funding_payments", nil, &params)
	if err != nil {
		return nil, err
	}
	// in Close()
	err = decode(res, &futures)
	if err != nil {
		return nil, err
	}
	return futures, nil
}
