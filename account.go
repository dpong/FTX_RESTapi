package api

import (
	"net/http"
)

type Account struct {
	Success bool `json:"success"`
	Result  struct {
		BackstopProvider             bool         `json:"backstopProvider"`
		Collateral                   float64      `json:"collateral"`
		FreeCollateral               float64      `json:"freeCollateral"`
		InitialMarginRequirement     float64      `json:"initialMarginRequirement"`
		Leverage                     float64      `json:"leverage"`
		Liquidating                  bool         `json:"liquidating"`
		MaintenanceMarginRequirement float64      `json:"maintenanceMarginRequirement"`
		MakerFee                     float64      `json:"makerFee"`
		MarginFraction               float64      `json:"marginFraction"`
		OpenMarginFraction           float64      `json:"openMarginFraction"`
		TakerFee                     float64      `json:"takerFee"`
		TotalAccountValue            float64      `json:"totalAccountValue"`
		TotalPositionSize            float64      `json:"totalPositionSize"`
		Username                     string       `json:"username"`
		Positions                    []*Positions `json:"positions"`
	} `json:"result"`
}

func (p *Client) Account() (account *Account) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/account",
		nil, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &account)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return account
}

type ResponseByLeverageChanged struct {
	Result  interface{} `json:"result"`
	Success bool        `json:"success"`
}

func (p *Client) Leverage(n int) (leverage interface{}) {
	params := make(map[string]interface{})
	params["leverage"] = n
	body, err := json.Marshal(params)
	if err != nil {
		p.Logger.Println(err)
		return 0
	}
	res, err := p.sendRequest(
		http.MethodPost,
		"/account/leverage",
		body, nil)
	if err != nil {
		p.Logger.Println(err)
		return 0
	}
	// in Close()
	err = decode(res, &leverage)
	if err != nil {
		p.Logger.Println(err)
		return 0
	}
	return leverage
}

type Balance struct {
	Success bool `json:"success"`
	Result  []struct {
		Coin                   string  `json:"coin"`
		Free                   float64 `json:"free"`
		Total                  float64 `json:"total"`
		AvailableWithoutBorrow float64 `json:"availableWithoutBorrow"`
		Borrowed               float64 `json:"spotBorrow"`
		USDValue               float64 `json:"usdValue"`
	} `json:"result"`
}

func (p *Client) Balances() (balances *Balance) {
	res, err := p.sendRequest(
		http.MethodGet,
		"/wallet/balances",
		nil, nil)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	// in Close()
	err = decode(res, &balances)
	if err != nil {
		p.Logger.Println(err)
		return nil
	}
	return balances
}
