package bitfinex

import (
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"

	. "github.com/nntaoli-project/goex"
)

const subscribe = "subscribe"
const subscribed = "subscribed"
const ticker = "ticker"
const trades = "trades"

type BitfinexWs struct {
	*WsBuilder
	sync.Once
	wsConn   *WsConn
	eventMap map[int64]SubscribeEvent

	tickerCallback func(*Ticker)
	tradeCallback  func(*Trade)
}

type SubscribeEvent struct {
	Event     string `json:"event"`
	SubID     string `json:"subId"`
	Channel   string `json:"channel"`
	ChanID    int64  `json:"chanId"`
	Symbol    string `json:"symbol"`
	Precision string `json:"prec,omitempty"`
	Frequency string `json:"freq,omitempty"`
	Key       string `json:"key,omitempty"`
	Len       string `json:"len,omitempty"`
	Pair      string `json:"pair"`
}

type EventMap map[int64]SubscribeEvent

func NewWs() *BitfinexWs {
	bws := &BitfinexWs{WsBuilder: NewWsBuilder(), eventMap: make(map[int64]SubscribeEvent)}
	bws.WsBuilder = bws.WsBuilder.
		WsUrl("wss://api-pub.bitfinex.com/ws/2").
		AutoReconnect().
		ProtoHandleFunc(bws.handle)
	return bws
}

func (bws *BitfinexWs) SetCallbacks(tickerCallback func(*Ticker), tradeCallback func(*Trade)) {
	bws.tickerCallback = tickerCallback
	bws.tradeCallback = tradeCallback
}

func (bws *BitfinexWs) SubscribeTicker(pair CurrencyPair) error {
	if bws.tickerCallback == nil {
		return errors.New("please set ticker callback func")
	}
	return bws.subscribe(map[string]interface{}{
		"event":   subscribe,
		"channel": ticker,
		"symbol":  convertPairToBitfinexSymbol(pair)})
}

func (bws *BitfinexWs) SubscribeTrade(pair CurrencyPair) error {
	if bws.tradeCallback == nil {
		return errors.New("please set trade callback func")
	}
	return bws.subscribe(map[string]interface{}{
		"event":   subscribe,
		"channel": trades,
		"symbol":  convertPairToBitfinexSymbol(pair)})
}

func (bws *BitfinexWs) subscribe(sub map[string]interface{}) error {
	bws.connectWs()
	return bws.wsConn.Subscribe(sub)
}

func (bws *BitfinexWs) connectWs() {
	bws.Do(func() {
		bws.wsConn = bws.WsBuilder.Build()
	})
}

func (bws *BitfinexWs) handle(msg []byte) error {
	var event SubscribeEvent
	if err := json.Unmarshal(msg, &event); err == nil {
		if event.Event == subscribed {
			bws.eventMap[event.ChanID] = event
			return nil
		}
	}

	var resp []interface{}
	if err := json.Unmarshal(msg, &resp); err == nil {
		channelID := ToInt64(resp[0])
		event, ok := bws.eventMap[channelID]
		if !ok {
			return nil
		}

		switch event.Channel {
		case ticker:
			if rawTicker, ok := resp[1].([]interface{}); ok {
				pair := symbolToCurrencyPair(event.Pair)
				t := bws.tickerFromRaw(pair, rawTicker)
				bws.tickerCallback(t)
				return nil
			}
		case trades:
			if len(resp) < 3 {
				return nil
			}

			if rawTrades, ok := resp[2].([]interface{}); ok {
				pair := symbolToCurrencyPair(event.Pair)
				trade := bws.tradeFromRaw(pair, rawTrades)
				bws.tradeCallback(trade)
				return nil
			}
		}

	}

	return nil
}

func (bws *BitfinexWs) tickerFromRaw(pair CurrencyPair, raw []interface{}) *Ticker {
	return &Ticker{
		Pair: pair,
		Buy:  ToFloat64(raw[0]),
		Sell: ToFloat64(raw[2]),
		Last: ToFloat64(raw[6]),
		Vol:  ToFloat64(raw[7]),
		High: ToFloat64(raw[8]),
		Low:  ToFloat64(raw[9]),
		Date: uint64(time.Now().UnixNano() / int64(time.Millisecond)),
	}
}

func (bws *BitfinexWs) tradeFromRaw(pair CurrencyPair, raw []interface{}) *Trade {

	amount := ToFloat64(raw[2])
	var side TradeSide
	if amount > 0 {
		side = BUY
	} else {
		side = SELL
	}

	return &Trade{
		Pair:   pair,
		Tid:    ToInt64(raw[0]),
		Date:   ToInt64(raw[1]),
		Amount: math.Abs(amount),
		Price:  ToFloat64(raw[3]),
		Type:   side,
	}
}

func convertPairToBitfinexSymbol(pair CurrencyPair) string {
	symbol := pair.ToSymbol("")
	return "t" + symbol
}
