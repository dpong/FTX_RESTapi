package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

const NullPrice = "null"

type StreamTickerBranch struct {
	bid    tobBranch
	ask    tobBranch
	cancel *context.CancelFunc
	reCh   chan error
}

type tobBranch struct {
	mux       sync.RWMutex
	price     string
	qty       string
	timeStamp time.Time
}

func StreamTicker(symbol string, logger *log.Logger) *StreamTickerBranch {
	var s StreamTickerBranch
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = &cancel
	ticker := make(chan map[string]interface{}, 50)
	errCh := make(chan error, 5)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := fTXTickerSocket(ctx, symbol, logger, &ticker, &errCh); err == nil {
					return
				} else {
					logger.Warningf("Reconnect %s ticker stream with err: %s\n", symbol, err.Error())
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := s.maintainStreamTicker(ctx, symbol, &ticker, &errCh); err == nil {
					return
				} else {
					logger.Warningf("Refreshing %s ticker stream with err: %s\n", symbol, err.Error())
				}
			}
		}
	}()
	return &s
}

func (s *StreamTickerBranch) Close() {
	(*s.cancel)()
	s.bid.mux.Lock()
	s.bid.price = NullPrice
	s.bid.mux.Unlock()
	s.ask.mux.Lock()
	s.ask.price = NullPrice
	s.ask.mux.Unlock()
}

func (s *StreamTickerBranch) GetBid() (price, qty string, timeStamp time.Time, ok bool) {
	s.bid.mux.RLock()
	defer s.bid.mux.RUnlock()
	price = s.bid.price
	qty = s.bid.qty
	timeStamp = s.bid.timeStamp
	if price == NullPrice || price == "" {
		return price, qty, timeStamp, false
	}
	return price, qty, timeStamp, true
}

func (s *StreamTickerBranch) GetAsk() (price, qty string, timeStamp time.Time, ok bool) {
	s.ask.mux.RLock()
	defer s.ask.mux.RUnlock()
	price = s.ask.price
	qty = s.ask.qty
	timeStamp = s.ask.timeStamp
	if price == NullPrice || price == "" {
		return price, qty, timeStamp, false
	}
	return price, qty, timeStamp, true
}

func (s *StreamTickerBranch) updateBidData(price, qty string, timeStamp time.Time) {
	s.bid.mux.Lock()
	defer s.bid.mux.Unlock()
	s.bid.price = price
	s.bid.qty = qty
	s.bid.timeStamp = timeStamp
}

func (s *StreamTickerBranch) updateAskData(price, qty string, timeStamp time.Time) {
	s.ask.mux.Lock()
	defer s.ask.mux.Unlock()
	s.ask.price = price
	s.ask.qty = qty
	s.ask.timeStamp = timeStamp
}

func (s *StreamTickerBranch) maintainStreamTicker(
	ctx context.Context,
	symbol string,
	ticker *chan map[string]interface{},
	errCh *chan error,
) error {
	lastUpdate := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case message := <-(*ticker):
			// millisecond level
			ts := time.UnixMicro(int64(message["time"].(float64) * 1000000))
			var bidPrice, askPrice, bidQty, askQty string
			if bid, ok := message["bid"].(float64); ok {
				bidDec := decimal.NewFromFloat(bid)
				bidPrice = bidDec.String()
			} else {
				bidPrice = NullPrice
			}
			if ask, ok := message["ask"].(float64); ok {
				askDec := decimal.NewFromFloat(ask)
				askPrice = askDec.String()
			} else {
				askPrice = NullPrice
			}
			if bidqty, ok := message["bidSize"].(float64); ok {
				bidQtyDec := decimal.NewFromFloat(bidqty)
				bidQty = bidQtyDec.String()
			}
			if askqty, ok := message["askSize"].(float64); ok {
				askQtyDec := decimal.NewFromFloat(askqty)
				askQty = askQtyDec.String()
			}
			s.updateBidData(bidPrice, bidQty, ts)
			s.updateAskData(askPrice, askQty, ts)
			lastUpdate = time.Now()
		default:
			if time.Now().After(lastUpdate.Add(time.Second * 10)) {
				// 10 sec without updating
				err := errors.New("reconnect because of time out")
				*errCh <- err
				return err
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func fTXTickerSocket(
	ctx context.Context,
	symbol string,
	logger *log.Logger,
	mainCh *chan map[string]interface{},
	reCh *chan error,
) error {
	var w fTXWebsocket
	var duration time.Duration = 300
	w.Logger = logger
	w.OnErr = false
	symbol = strings.ToUpper(symbol)
	url := "wss://ftx.com/ws/"
	// wait 5 second, if the hand shake fail, will terminate the dail
	dailCtx, _ := context.WithDeadline(ctx, time.Now().Add(time.Second*5))
	conn, _, err := websocket.DefaultDialer.DialContext(dailCtx, url, nil)
	if err != nil {
		return err
	}
	logger.Infof("FTX %s orderBook socket connected.\n", symbol)
	w.Conn = conn
	defer conn.Close()

	send, err := getFTXTickerSubscribeMessage(symbol)
	if err != nil {
		return err
	}
	if err := w.Conn.WriteMessage(websocket.TextMessage, send); err != nil {
		return err
	}
	if err := w.Conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-(*reCh):
			return err
		default:
			if conn == nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message)
				return errors.New(message)
			}
			_, buf, err := conn.ReadMessage()
			if err != nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message)
				return errors.New(message)
			}
			res, err1 := fTXDecoding(&buf)
			if err1 != nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message, err1)
				return err1
			}
			err2 := handleFTXWebsocket(&res, mainCh)
			if err2 != nil {
				d := w.OutFTXErr()
				*mainCh <- d
				message := "FTX reconnect..."
				logger.Infoln(message)
				return err2
			}
			if err := w.Conn.SetReadDeadline(time.Now().Add(time.Second * duration)); err != nil {
				return err
			}
		}
	}
}

func getFTXTickerSubscribeMessage(market string) ([]byte, error) {
	sub := fTXSubscribeMessage{Op: "subscribe", Channel: "ticker", Market: market}
	message, err := json.Marshal(sub)
	if err != nil {
		return nil, err
	}
	return message, nil
}
