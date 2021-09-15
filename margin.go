package api

import (
	"bytes"
	"net/http"
	"time"
)

type GetBorrowRatesResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Coin     string  `json:"coin"`
		Estimate float64 `json:"estimate"`
		Previous float64 `json:"previous"`
	} `json:"result"`
}

func (p *Client) GetBorrowRates() (borrow *GetBorrowRatesResponse, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/spot_margin/borrow_rates",
		nil, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &borrow)
	if err != nil {
		return nil, err
	}
	return borrow, nil
}

type GetLendingRatesResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Coin     string  `json:"coin"`
		Estimate float64 `json:"estimate"`
		Previous float64 `json:"previous"`
	} `json:"result"`
}

func (p *Client) GetLendingRates() (lending *GetLendingRatesResponse, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/spot_margin/lending_rates",
		nil, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &lending)
	if err != nil {
		return nil, err
	}
	return lending, nil
}

type LendingHistoryResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Coin     string    `json:"coin"`
		Proceeds float64   `json:"proceeds"`
		Rate     float64   `json:"rate"`
		Size     float64   `json:"size"`
		Time     time.Time `json:"time"`
	} `json:"result"`
}

func (p *Client) GetBorrowHistory() (result *BorrowHistoryResponse, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/spot_margin/borrow_history",
		nil, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type BorrowHistoryResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		Coin string    `json:"coin"`
		Cost float64   `json:"cost"`
		Rate float64   `json:"rate"`
		Size float64   `json:"size"`
		Time time.Time `json:"time"`
	} `json:"result"`
}

func (p *Client) GetLendingHistory() (result *LendingHistoryResponse, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/spot_margin/lending_history",
		nil, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type LendingInfoResponse struct {
	Success bool                 `json:"success"`
	Result  []LendingInfoResults `json:"result"`
}

type LendingInfoResults struct {
	Coin     string  `json:"coin"`
	Lendable float64 `json:"lendable"`
	Locked   float64 `json:"locked"`
	MinRate  float64 `json:"minRate"`
	Offered  float64 `json:"offered"`
}

func (p *Client) GetLendingInfo() (result *LendingInfoResponse, err error) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/spot_margin/lending_info",
		nil, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type MarginMarketInfoResults struct {
	Success bool `json:"success"`
	Result  []struct {
		Coin          string  `json:"coin"`
		Borrowed      float64 `json:"borrowed"`
		Free          float64 `json:"free"`
		EstimatedRate float64 `json:"estimatedRate"`
		PreviousRate  float64 `json:"previousRate"`
	} `json:"result"`
}

func (p *Client) GetMarginMarketInfo(symbol string) (result *MarginMarketInfoResults, err error) {
	var buffer bytes.Buffer
	buffer.WriteString("/spot_margin/market_info?market=")
	buffer.WriteString(symbol)
	res, err := p.sendRequest(
		http.MethodGet,
		buffer.String(),
		nil, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type SubmitLendingOfferResponse struct {
	Success bool        `json:"success"`
	Result  interface{} `json:"result"`
}

func (p *Client) SubmitLendingOffer(coin string, amount, rate float64) (result *SubmitLendingOfferResponse, err error) {
	params := make(map[string]interface{})
	params["coin"] = coin
	params["size"] = amount
	params["rate"] = rate
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/spot_margin/offers",
		body, nil)
	if err != nil {
		return nil, err
	}
	err = decode(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
