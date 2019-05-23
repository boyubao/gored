package kraken

// Copyright (c) 2015-2019 Bitontop Technologies Inc.
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bitontop/gored/coin"
	"github.com/bitontop/gored/exchange"
	"github.com/bitontop/gored/pair"
)

const (
	API_URL string = "https://api.kraken.com/0"
)

/*API Base Knowledge
Path: API function. Usually after the base endpoint URL
Method:
	Get - Call a URL, API return a response
	Post - Call a URL & send a request, API return a response
Public API:
	It doesn't need authorization/signature , can be called by browser to get response.
	using exchange.HttpGetRequest/exchange.HttpPostRequest
Private API:
	Authorization/Signature is requried. The signature request should look at Exchange API Document.
	using ApiKeyGet/ApiKeyPost
Response:
	Response is a json structure.
	Copy the json to https://transform.now.sh/json-to-go/ convert to go Struct.
	Add the go Struct to model.go

ex. Get /api/v1/depth
Get - Method
/api/v1/depth - Path*/

/*************** Public API ***************/
/*Get Coins Information (If API provide)
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Modify API Path(strRequestUrl)*/
func (e *Kraken) GetCoinsData() {
	jsonResponse := &JsonResponse{}
	coinsData := make(map[string]*CoinsData)

	strRequestUrl := "/public/Assets"
	strUrl := API_URL + strRequestUrl

	jsonCurrencyReturn := exchange.HttpGetRequest(strUrl, nil)
	if err := json.Unmarshal([]byte(jsonCurrencyReturn), &jsonResponse); err != nil {
		log.Printf("%s Get Coins Json Unmarshal Err: %v %v", e.GetName(), err, jsonCurrencyReturn)
	} else if len(jsonResponse.Error) != 0 {
		log.Printf("%s Get Coins Failed: %v", e.GetName(), jsonResponse.Error)
		return
	}
	if err := json.Unmarshal(jsonResponse.Result, &coinsData); err != nil {
		log.Printf("%s Get Coins Result Unmarshal Err: %v %s", e.GetName(), err, jsonResponse.Result)
	}

	for key, data := range coinsData {
		c := &coin.Coin{}
		switch e.Source {
		case exchange.EXCHANGE_API:
			c = coin.GetCoin(data.Altname)
			if c == nil {
				c = &coin.Coin{}
				c.Code = data.Altname
				coin.AddCoin(c)
			}
		case exchange.JSON_FILE:
			c = e.GetCoinBySymbol(data.Altname)
		}

		if c != nil {
			coinConstraint := &exchange.CoinConstraint{
				CoinID:       c.ID,
				Coin:         c,
				ExSymbol:     key,
				TxFee:        DEFAULT_TXFEE,
				Withdraw:     DEFAULT_WITHDRAW,
				Deposit:      DEFAULT_DEPOSIT,
				Confirmation: DEFAULT_CONFIRMATION,
				Listed:       DEFAULT_LISTED,
			}
			e.SetCoinConstraint(coinConstraint)
		}
	}
}

/* GetPairsData - Get Pairs Information (If API provide)
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Modify API Path(strRequestUrl)*/
func (e *Kraken) GetPairsData() {
	jsonResponse := &JsonResponse{}
	pairsData := make(map[string]*PairsData)

	strRequestUrl := "/public/AssetPairs"
	strUrl := API_URL + strRequestUrl

	jsonSymbolsReturn := exchange.HttpGetRequest(strUrl, nil)
	if err := json.Unmarshal([]byte(jsonSymbolsReturn), &jsonResponse); err != nil {
		log.Printf("%s Get Pairs Json Unmarshal Err: %v %v", e.GetName(), err, jsonSymbolsReturn)
	} else if len(jsonResponse.Error) != 0 {
		log.Printf("%s Get Pairs Failed: %v", e.GetName(), jsonResponse.Error)
	}
	if err := json.Unmarshal(jsonResponse.Result, &pairsData); err != nil {
		log.Printf("%s Get Pairs Result Unmarshal Err: %v %s", e.GetName(), err, jsonResponse.Result)
	}

	for key, data := range pairsData {
		ch := strings.Split(key, ".")
		if len(ch) == 1 {
			p := &pair.Pair{}
			switch e.Source {
			case exchange.EXCHANGE_API:
				base := e.GetCoinBySymbol(data.Quote)
				target := e.GetCoinBySymbol(data.Base)
				if base != nil && target != nil {
					p = pair.GetPair(base, target)
				}
			case exchange.JSON_FILE:
				p = e.GetPairBySymbol(key)
			}
			if p != nil {
				pairConstraint := &exchange.PairConstraint{
					PairID:      p.ID,
					Pair:        p,
					ExSymbol:    key,
					LotSize:     math.Pow10(-1 * data.LotDecimals),
					PriceFilter: math.Pow10(-1 * data.PairDecimals),
					Listed:      DEFAULT_LISTED,
				}
				if len(data.FeesMaker) >= 1 {
					pairConstraint.MakerFee = data.FeesMaker[0][1]
				}
				if len(data.Fees) >= 1 {
					pairConstraint.TakerFee = data.Fees[0][1]
				}
				e.SetPairConstraint(pairConstraint)
			}
		}
	}
}

/*Get Pair Market Depth
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Add Model of API Response
Step 3: Get Exchange Pair Code ex. symbol := e.GetPairCode(p)
Step 4: Modify API Path(strRequestUrl)
Step 5: Add Params - Depend on API request
Step 6: Convert the response to Standard Maker struct*/
func (e *Kraken) OrderBook(pair *pair.Pair) (*exchange.Maker, error) {
	jsonResponse := &JsonResponse{}
	orderBook := make(map[string]*OrderBook)
	symbol := e.GetSymbolByPair(pair)

	mapParams := make(map[string]string)
	mapParams["pair"] = symbol
	mapParams["count"] = "100"

	strRequestUrl := "/public/Depth"
	strUrl := API_URL + strRequestUrl

	maker := &exchange.Maker{}
	maker.WorkerIP = exchange.GetExternalIP()
	maker.BeforeTimestamp = float64(time.Now().UnixNano() / 1e6)

	jsonOrderbook := exchange.HttpGetRequest(strUrl, mapParams)
	if err := json.Unmarshal([]byte(jsonOrderbook), &jsonResponse); err != nil {
		return nil, fmt.Errorf("%s Get Orderbook Json Unmarshal Err: %v %v", e.GetName(), err, jsonOrderbook)
	} else if len(jsonResponse.Error) != 0 {
		return nil, fmt.Errorf("%s Get Orderbook Failed: %v", e.GetName(), jsonResponse.Error)
	}
	if err := json.Unmarshal(jsonResponse.Result, &orderBook); err != nil {
		return nil, fmt.Errorf("%s Get Orderbook Result Unmarshal Err: %v %s", e.GetName(), err, jsonResponse.Result)
	}

	maker.AfterTimestamp = float64(time.Now().UnixNano() / 1e6)
	var err error
	for _, book := range orderBook {
		for _, bid := range book.Bids {
			buydata := exchange.Order{}
			buydata.Quantity, err = strconv.ParseFloat(bid[1].(string), 64)
			if err != nil {
				return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
			}

			buydata.Rate, err = strconv.ParseFloat(bid[0].(string), 64)
			if err != nil {
				return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
			}
			maker.Bids = append(maker.Bids, buydata)
		}
	}
	for _, book := range orderBook {
		for _, ask := range book.Asks {
			selldata := exchange.Order{}
			selldata.Quantity, err = strconv.ParseFloat(ask[1].(string), 64)
			if err != nil {
				return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
			}

			selldata.Rate, err = strconv.ParseFloat(ask[0].(string), 64)
			if err != nil {
				return nil, fmt.Errorf("%s OrderBook strconv.ParseFloat Quantity error:%v", e.GetName(), err)
			}
			maker.Asks = append(maker.Asks, selldata)
		}
	}
	return maker, nil
}

/*************** Private API ***************/
func (e *Kraken) UpdateAllBalances() {

}

func (e *Kraken) Withdraw(coin *coin.Coin, quantity float64, addr, tag string) bool {

	return false
}

func (e *Kraken) LimitSell(pair *pair.Pair, quantity, rate float64) (*exchange.Order, error) {

	return nil, nil
}

func (e *Kraken) LimitBuy(pair *pair.Pair, quantity, rate float64) (*exchange.Order, error) {

	return nil, nil
}

func (e *Kraken) OrderStatus(order *exchange.Order) error {

	return nil
}

func (e *Kraken) ListOrders() ([]*exchange.Order, error) {
	return nil, nil
}

func (e *Kraken) CancelOrder(order *exchange.Order) error {

	return nil
}

func (e *Kraken) CancelAllOrder() error {
	return nil
}

/*************** Signature Http Request ***************/
/*Method: API Request and Signature is required
Step 1: Change Instance Name    (e *<exchange Instance Name>)
Step 2: Create mapParams Depend on API Signature request
Step 3: Add HttpGetRequest below strUrl if API has different requests*/
func (e *Kraken) ApiKeyGET(strRequestPath string, mapParams map[string]string) string {
	mapParams["apikey"] = e.API_KEY
	mapParams["nonce"] = fmt.Sprintf("%d", time.Now().UnixNano())

	strUrl := API_URL + strRequestPath + "?" + exchange.Map2UrlQuery(mapParams)

	signature := exchange.ComputeHmac512NoDecode(strUrl, e.API_SECRET)
	httpClient := &http.Client{}

	request, err := http.NewRequest("GET", strUrl, nil)
	if nil != err {
		return err.Error()
	}
	request.Header.Add("Content-Type", "application/json;charset=utf-8")
	request.Header.Add("Accept", "application/json")
	request.Header.Add("apisign", signature)

	response, err := httpClient.Do(request)
	if nil != err {
		return err.Error()
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if nil != err {
		return err.Error()
	}

	return string(body)
}