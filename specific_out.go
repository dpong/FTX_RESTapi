package api

import (
	"strings"

	"github.com/shopspring/decimal"
)

// format: [][]string{asset, available, total}
func (c *Client) GetBalances() ([][]string, bool) {
	res, err := c.Balances()
	if err != nil {
		return [][]string{}, false
	}
	var result [][]string
	for _, item := range res.Result {
		free := decimal.NewFromFloat(item.AvailableWithoutBorrow)
		total := decimal.NewFromFloat(item.Total)
		data := []string{item.Coin, free.String(), total.String()}
		result = append(result, data)
	}
	return result, true
}

// format: [][]string{oid, symbol, product, subaccount, price, qty, side, orderType, UnfilledQty}
func (c *Client) GetOpenOrders() ([][]string, bool) {
	res, err := c.GetOpenOrdersWithSymbol("")
	if err != nil {
		return [][]string{}, false
	}
	var result [][]string
	for _, item := range res.Result {
		oid := decimal.NewFromInt(int64(item.ID)).String()
		origQty := decimal.NewFromFloat(item.Size)
		//filledQty := decimal.NewFromFloat(item.Filledsiz)e
		unfilledQty := decimal.NewFromFloat(item.Remainingsize)
		price := decimal.NewFromFloat(item.Price)
		data := []string{oid, item.Market, "spot", c.subaccount, price.String(), origQty.String(), item.Side, item.Type, unfilledQty.String()}
		result = append(result, data)
	}
	return result, true
}

// [][]string{oid, symbol, product, subaccount, price, qty, side, orderType, fee, filledQty, timestamp, isMaker}
func (c *Client) GetTradeReports() ([][]string, bool) {
	var result [][]string
	for {
		trades, err := c.userTrade.ReadTrade()
		if err != nil {
			break
		}
		for _, trade := range trades {
			var isMaker string

			if trade.IsMaker {
				isMaker = "true"
			} else {
				isMaker = "false"
			}
			st := decimal.NewFromInt(trade.TimeStamp.Unix()).String()
			data := []string{trade.Oid, trade.Symbol, "spot", c.subaccount, trade.Price.String(), trade.Qty.String(), trade.Side, strings.ToLower(trade.OrderType), trade.Fee.String(), trade.Qty.String(), st, isMaker}
			result = append(result, data)
		}
	}
	if len(result) == 0 {
		return result, false
	}
	return result, true
}
