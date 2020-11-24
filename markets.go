package api

import (
	"fmt"
	"net/http"
	"time"
)

type Market struct {
	Success bool `json:"success"`
	Result  []struct {
		Name           string  `json:"name"`
		BaseCurrency   string  `json:"baseCurrency"`
		QuoteCurrency  string  `json:"quoteCurrency"`
		Type           string  `json:"type"`
		Underlying     string  `json:"underlying"`
		Enabled        bool    `json:"enabled"`
		Ask            float64 `json:"ask"`
		Bid            float64 `json:"bid"`
		Last           float64 `json:"last"`
		PriceIncrement float64 `json:"priceIncrement"`
		SizeIncrement  float64 `json:"sizeIncrement"`
		Restricted     bool    `json:"restricted"`
	} `json:"result"`
}

type Future struct {
	Success bool `json:"success"`
	Result  []struct {
		Ask                 float64 `json:"ask"`
		Bid                 float64 `json:"bid"`
		Change1H            float64 `json:"change1h"`
		Change24H           float64 `json:"change24h"`
		ChangeBod           float64 `json:"changeBod"`
		VolumeUsd24H        float64 `json:"volumeUsd24h"`
		Volume              float64 `json:"volume"`
		Description         string  `json:"description"`
		Enabled             bool    `json:"enabled"`
		Expired             bool    `json:"expired"`
		Expiry              string  `json:"expiry"`
		Index               float64 `json:"index"`
		Last                float64 `json:"last"`
		LowerBound          float64 `json:"lowerBound"`
		Mark                float64 `json:"mark"`
		Name                string  `json:"name"`
		Perpetual           bool    `json:"perpetual"`
		PositionLimitWeight float64 `json:"positionLimitWeight"`
		PostOnly            bool    `json:"postOnly"`
		PriceIncrement      float64 `json:"priceIncrement"`
		SizeIncrement       float64 `json:"sizeIncrement"`
		Underlying          string  `json:"underlying"`
		UpperBound          float64 `json:"upperBound"`
		Type                string  `json:"type"`
	} `json:"result"`
}

func (p *Client) Markets() (markets *Market) {
	res, err := p.sendRequest(http.MethodGet, "/markets", nil, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &markets)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}

	return markets
}

func (p *Client) Futures() (futures *Future) {
	res, err := p.sendRequest(http.MethodGet, "/futures", nil, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &futures)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return futures
}

func (p *Client) FutureStats(symbol string) (futures *FutureStatsResponse) {
	path := fmt.Sprintf("/futures/%s/stats", symbol)
	res, err := p.sendRequest(http.MethodGet, path, nil, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &futures)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return futures
}

type FutureStatsResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Volume                   float64   `json:"volume"`
		NextFundingRate          float64   `json:"nextFundingRate"`
		NextFundingTime          time.Time `json:"nextFundingTime"`
		ExpirationPrice          float64   `json:"expirationPrice"`
		PredictedExpirationPrice float64   `json:"predictedExpirationPrice"`
		StrikePrice              float64   `json:"strikePrice"`
		OpenInterest             float64   `json:"openInterest"`
	} `json:"result"`
}
