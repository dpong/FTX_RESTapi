package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

type StreamMarketTradesBranch struct {
	cancel *context.CancelFunc
	conn   *websocket.Conn
	market string

	tradeChan    chan FTXTradeData
	tradesBranch struct {
		Trades []FTXTradeData
		sync.Mutex
	}
	logger *logrus.Logger
}

type FTXTradeData struct {
	Channel string `json:"channel"`
	Market  string `json:"market"`
	Type    string `json:"type"`
	Data    []struct {
		ID          int64     `json:"id"`
		Price       float64   `json:"price"`
		Size        float64   `json:"size"`
		Side        string    `json:"side"`
		Liquidation bool      `json:"liquidation"`
		Time        time.Time `json:"time"`
	} `json:"data"`
}

func TradeStream(symbol string, logger *logrus.Logger) *StreamMarketTradesBranch {
	Usymbol := strings.ToUpper(symbol)
	return tradeStream(Usymbol, logger)
}

// side: Side of the taker in the trade
func (o *StreamMarketTradesBranch) GetTrades() []FTXTradeData {
	o.tradesBranch.Lock()
	defer o.tradesBranch.Unlock()
	trades := o.tradesBranch.Trades
	o.tradesBranch.Trades = []FTXTradeData{}
	return trades
}

func (o *StreamMarketTradesBranch) Close() {
	(*o.cancel)()
	o.tradesBranch.Lock()
	defer o.tradesBranch.Unlock()
	o.tradesBranch.Trades = []FTXTradeData{}
}

func tradeStream(symbol string, logger *logrus.Logger) *StreamMarketTradesBranch {
	o := new(StreamMarketTradesBranch)
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = &cancel
	o.market = symbol
	o.tradeChan = make(chan FTXTradeData, 100)
	o.logger = logger
	go o.maintainSession(ctx, symbol)
	go o.listen(ctx)
	return o
}

func (o *StreamMarketTradesBranch) listen(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case trade := <-o.tradeChan:
			o.appendNewTrade(&trade)
		default:
			time.Sleep(time.Second)
		}
	}
}

func (o *StreamMarketTradesBranch) appendNewTrade(new *FTXTradeData) {
	o.tradesBranch.Lock()
	defer o.tradesBranch.Unlock()
	o.tradesBranch.Trades = append(o.tradesBranch.Trades, *new)
}

func (o *StreamMarketTradesBranch) maintainSession(ctx context.Context, symbol string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := o.maintain(ctx, symbol); err == nil {
				return
			} else {
				o.logger.Warningf("reconnect FTX %s trade stream with err: %s\n", symbol, err.Error())
			}
		}
	}
}

func (o *StreamMarketTradesBranch) maintain(ctx context.Context, symbol string) error {
	var duration time.Duration = 30
	innerErr := make(chan error, 1)
	url := "wss://ftx.com/ws/"
	// wait 5 second, if the hand shake fail, will terminate the dail
	dailCtx, _ := context.WithDeadline(ctx, time.Now().Add(time.Second*5))
	conn, _, err := websocket.DefaultDialer.DialContext(dailCtx, url, nil)
	if err != nil {
		return err
	}
	o.conn = conn
	defer o.conn.Close()
	if send, err := o.getSubscribeMessage(); err != nil {
		return err
	} else {
		if err := o.conn.WriteMessage(websocket.TextMessage, send); err != nil {
			return err
		}
	}
	if err := o.conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
		return err
	}
	o.conn.SetPingHandler(nil)
	go func() {
		PingManaging := time.NewTicker(time.Second * 15)
		defer PingManaging.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-innerErr:
				return
			case <-PingManaging.C:
				send := getPingPong()
				if err := o.conn.WriteMessage(websocket.TextMessage, send); err != nil {
					o.conn.SetReadDeadline(time.Now().Add(time.Second))
					return
				}
				o.conn.SetReadDeadline(time.Now().Add(time.Second * duration))
			default:
				time.Sleep(time.Second)
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, msg, err := o.conn.ReadMessage()
			if err != nil {
				innerErr <- errors.New("restart")
				return err
			}
			if err := o.handleFTXTradeSocketMsg(msg); err != nil {
				innerErr <- errors.New("restart")
				return err
			}
			if err := o.conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
				innerErr <- errors.New("restart")
				return err
			}
		} // end select
	} // end for
}

func (o *StreamMarketTradesBranch) handleFTXTradeSocketMsg(msg []byte) error {
	var data FTXTradeData
	err := json.Unmarshal(msg, &data)
	if err != nil {
		return errors.New("fail to unmarshal message")
	}
	switch data.Channel {
	case "trades":
		switch data.Type {
		case "update":
			o.tradeChan <- data
		case "subscribed":
			o.logger.Infof("Connected FTX %s trade stream.", o.market)
		default:
			// error and info
			return err
		}
	default:
		// ping pong
	}
	return nil
}

func (o *StreamMarketTradesBranch) getSubscribeMessage() ([]byte, error) {
	sub := fTXSubscribeMessage{Op: "subscribe", Channel: "trades", Market: o.market}
	message, err := json.Marshal(sub)
	if err != nil {
		return nil, err
	}
	return message, nil
}
