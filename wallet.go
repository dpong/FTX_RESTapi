package api

import (
	"net/http"
)

type TransferInSubaccountOpts struct {
	Coin        string  `json:"coin"`
	Size        float64 `json:"size"`
	Source      string  `json:"source"`
	Destination string  `json:"destination"`
}

type TransferInSubaccountResponse struct {
	Success bool `json:"success"`
	Result  struct {
		ID     int     `json:"id"`
		Coin   string  `json:"coin"`
		Size   float64 `json:"size"`
		Time   string  `json:"time"`
		Notes  string  `json:"notes"`
		Status string  `json:"status"`
	} `json:"result"`
}

func (p *Client) TransferInSubaccounts(asset, from, to string, size float64) (transfer *TransferInSubaccountResponse) {
	params := make(map[string]interface{})
	params["coin"] = asset
	params["size"] = size
	params["source"] = from
	params["destination"] = to
	body, err := json.Marshal(params)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/subaccounts/transfer",
		body, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	err = decode(res, &transfer)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return transfer
}

type WithdrawOpts struct {
	Coin     string  `json:"coin"`
	Size     float64 `json:"size"`
	Address  string  `json:"address"`
	Tag      string  `json:"tag"`
	Password string  `json:"password"`
	Code     string  `json:"code"`
}

type WithdrawResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Coin    string  `json:"coin"`
		Address string  `json:"address"`
		Tag     string  `json:"tag"`
		Fee     float64 `json:"fee"`
		ID      int     `json:"id"`
		Size    float64 `json:"size"`
		Status  string  `json:"status"`
		Time    string  `json:"time"`
		Txid    string  `json:"txid"`
	} `json:"result"`
}

func (p *Client) Withdraw(asset, address, password string, size float64) (withdraw *WithdrawResponse) {
	params := make(map[string]interface{})
	params["coin"] = asset
	params["size"] = size
	params["address"] = address
	params["password"] = password
	body, err := json.Marshal(params)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/wallet/withdrawals",
		body, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	err = decode(res, &withdraw)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return withdraw
}
