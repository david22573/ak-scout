package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

)

type Candle struct {
	Market              string  `json:"market"`
	Symbol              string  `json:"symbol"`
	Interval            string  `json:"interval"`
	OpenTime            int64   `json:"open_time"`
	Open                float64 `json:"open"`
	High                float64 `json:"high"`
	Low                 float64 `json:"low"`
	Close               float64 `json:"close"`
	Volume              float64 `json:"volume"`
	CloseTime           int64   `json:"close_time"`
	QuoteAssetVolume    float64 `json:"quote_asset_volume"`
	NumberOfTrades      int64   `json:"number_of_trades"`
	TakerBuyBaseVolume  float64 `json:"taker_buy_base_volume"`
	TakerBuyQuoteVolume float64 `json:"taker_buy_quote_volume"`
}

// Data Structures for Snapshot

type TimeframeData struct {
	MA20         float64   `json:"ma20"`
	MA100        float64   `json:"ma100"`
	MA200        float64   `json:"ma200"`
	DistanceMA20 float64   `json:"distance_ma20"`
	DistanceMA10 float64   `json:"distance_ma100"`
	DistanceMA200 float64  `json:"distance_ma200"`
	RSI6         float64   `json:"rsi6"`
	KDJ          []float64 `json:"kdj"` // [K, D, J]
	ATR          float64   `json:"atr,omitempty"`
	VolumeRatio  float64   `json:"volume_ratio,omitempty"`
	SwingHigh    float64   `json:"swing_high,omitempty"`
	SwingLow     float64   `json:"swing_low,omitempty"`
}

type DerivativesData struct {
	Funding             float64   `json:"funding"`
	FundingHistory      []float64 `json:"funding_history"`
	OpenInterest        float64   `json:"open_interest"`
	OIChange15m         float64   `json:"oi_change_15m"`
	OIChange1h          float64   `json:"oi_change_1h"`
	OIChange4h          float64   `json:"oi_change_4h"`
	OrderBookBidPercent float64   `json:"orderbook_bid_percent"`
	OrderBookAskPercent float64   `json:"orderbook_ask_percent"`
}

type AriDecision struct {
	LongConfidence  *float64 `json:"long_confidence"`
	ShortConfidence *float64 `json:"short_confidence"`
	WaitConfidence  *float64 `json:"wait_confidence"`
	Recommendation  *string  `json:"recommendation"`
	Reasoning       *string  `json:"reasoning"`
}

type FutureOutcome struct {
	After30m         *float64 `json:"after_30m"`
	After1h          *float64 `json:"after_1h"`
	After4h          *float64 `json:"after_4h"`
	MaxFavorableMove *float64 `json:"max_favorable_move"`
	MaxAdverseMove   *float64 `json:"max_adverse_move"`
}

type MarketSnapshot struct {
	Symbol            string                   `json:"symbol"`
	TimestampUTC      string                   `json:"timestamp_utc"`
	Price             float64                  `json:"price"`
	Timeframes        map[string]TimeframeData `json:"timeframes"`
	BTCContext        map[string]TimeframeData `json:"btc_context"`
	Derivatives       DerivativesData          `json:"derivatives"`
	BTCAgreementScore float64                  `json:"btc_agreement_score"`
	AriDecision       AriDecision              `json:"ari_decision"`
	FutureOutcome     FutureOutcome            `json:"future_outcome"`
}

// API Structures

type PremiumIndexItem struct {
	Symbol          string `json:"symbol"`
	Pair            string `json:"pair"`
	MarkPrice       string `json:"markPrice"`
	IndexPrice      string `json:"indexPrice"`
	LastFundingRate string `json:"lastFundingRate"`
	NextFundingTime int64  `json:"nextFundingTime"`
	Time            int64  `json:"time"`
}

type OpenInterestItem struct {
	Symbol       string `json:"symbol"`
	Pair         string `json:"pair"`
	OpenInterest string `json:"openInterest"`
	ContractType string `json:"contractType"`
	Time         int64  `json:"time"`
}

type OpenInterestHistItem struct {
	Pair                 string `json:"pair"`
	ContractType         string `json:"contractType"`
	SumOpenInterest      string `json:"sumOpenInterest"`
	SumOpenInterestValue string `json:"sumOpenInterestValue"`
	Timestamp            int64  `json:"timestamp"`
}

type DepthItem struct {
	Bids [][]string `json:"bids"`
	Asks [][]string `json:"asks"`
}

// Global Variables
var httpClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "snapshot":
		runSnapshot(os.Args[2:])
	case "record":
		runRecord(os.Args[2:])
	case "check-outcomes":
		runCheckOutcomes(os.Args[2:])
	case "report":
		runReport(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("ak-scout is a read-only Binance COIN-M market reader and decision journal tool.")
	fmt.Println("\nUsage:")
	fmt.Println("  ak-scout snapshot --symbol <symbol> --context <context_symbol>")
	fmt.Println("  ak-scout record --snapshot <json_path> --recommendation <LONG/SHORT/WAIT> --long-conf <0-10> --short-conf <0-10> --wait-conf <0-10> [--entry <price>] [--stop <price>] [--tp1 <price>] [--tp2 <price>] [--reason <reason>]")
	fmt.Println("  ak-scout check-outcomes")
	fmt.Println("  ak-scout report [--from <YYYY-MM-DD>] [--to <YYYY-MM-DD>]")
}

// --- Commands Implementation ---

func runSnapshot(args []string) {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	symbolOpt := fs.String("symbol", "BNBUSD_PERP", "Binance COIN-M symbol (e.g. BNBUSD_PERP)")
	contextOpt := fs.String("context", "BTCUSD_PERP", "Binance COIN-M context symbol (e.g. BTCUSD_PERP)")
	fs.Parse(args)

	symbol := strings.ToUpper(*symbolOpt)
	contextSymbol := strings.ToUpper(*contextOpt)

	fmt.Printf("⏳ Gathering COIN-M market data for %s (context: %s)...\n", symbol, contextSymbol)

	// Fetch current price & funding info
	premIndex, err := fetchPremiumIndex(symbol)
	if err != nil {
		logError("Failed to fetch premium index: %v", err)
		os.Exit(1)
	}

	currentPrice, err := strconv.ParseFloat(premIndex.MarkPrice, 64)
	if err != nil {
		logError("Failed to parse current price: %v", err)
		os.Exit(1)
	}

	funding, _ := strconv.ParseFloat(premIndex.LastFundingRate, 64)

	// Fetch funding rate history (limit 10)
	fundingHist, err := fetchFundingHistory(symbol, 10)
	var fundingHistoryList []float64
	if err == nil {
		for _, fh := range fundingHist {
			if r, e := strconv.ParseFloat(fh.FundingRate, 64); e == nil {
				fundingHistoryList = append(fundingHistoryList, r)
			}
		}
	}

	// Fetch order book depth
	depth, err := fetchOrderBook(symbol, 100)
	if err != nil {
		logError("Failed to fetch depth: %v", err)
		os.Exit(1)
	}
	bidPercent, askPercent := calculateOrderBookImbalance(depth)

	// Fetch open interest
	oi, err := fetchOpenInterest(symbol)
	var currentOI float64
	if err == nil {
		currentOI, _ = strconv.ParseFloat(oi.OpenInterest, 64)
	}

	// Fetch open interest history to calculate changes
	pair := strings.Split(symbol, "_")[0]
	oiHist, err := fetchOpenInterestHistory(pair, "15m", 30)
	var oiChange15m, oiChange1h, oiChange4h float64
	if err == nil && len(oiHist) > 0 {
		latestOIStr := oiHist[len(oiHist)-1].SumOpenInterest
		latestOI, err1 := strconv.ParseFloat(latestOIStr, 64)
		if err1 == nil {
			if len(oiHist) >= 2 {
				prev15m, _ := strconv.ParseFloat(oiHist[len(oiHist)-2].SumOpenInterest, 64)
				if prev15m > 0 {
					oiChange15m = ((latestOI - prev15m) / prev15m) * 100
				}
			}
			if len(oiHist) >= 5 {
				prev1h, _ := strconv.ParseFloat(oiHist[len(oiHist)-5].SumOpenInterest, 64)
				if prev1h > 0 {
					oiChange1h = ((latestOI - prev1h) / prev1h) * 100
				}
			}
			if len(oiHist) >= 17 {
				prev4h, _ := strconv.ParseFloat(oiHist[len(oiHist)-17].SumOpenInterest, 64)
				if prev4h > 0 {
					oiChange4h = ((latestOI - prev4h) / prev4h) * 100
				}
			}
		}
	}

	// Fetch candles for symbol
	symbolTimeframes := []string{"15m", "30m", "1h", "4h", "1d"}
	tfDataMap := make(map[string]TimeframeData)

	for _, tf := range symbolTimeframes {
		candles, err := fetchKlines(symbol, tf, 300)
		if err != nil {
			logError("Failed to fetch %s klines for %s: %v", tf, symbol, err)
			os.Exit(1)
		}
		tfDataMap[tf] = calculateTimeframeData(candles, currentPrice)
	}

	// Fetch candles for context symbol
	contextTimeframes := []string{"15m", "1h", "4h"}
	ctxPrice, err := fetchContextPrice(contextSymbol)
	if err != nil {
		logError("Failed to fetch context price for %s: %v", contextSymbol, err)
		os.Exit(1)
	}

	ctxDataMap := make(map[string]TimeframeData)
	for _, tf := range contextTimeframes {
		candles, err := fetchKlines(contextSymbol, tf, 300)
		if err != nil {
			logError("Failed to fetch %s klines for context symbol %s: %v", tf, contextSymbol, err)
			os.Exit(1)
		}
		ctxDataMap[tf] = calculateTimeframeData(candles, ctxPrice)
	}

	// Calculate BTC Agreement Score
	agreementScore := calculateAgreementScore(tfDataMap, ctxDataMap)

	// Build the MarketSnapshot object
	nowUTC := time.Now().UTC()
	snapshot := MarketSnapshot{
		Symbol:       symbol,
		TimestampUTC: nowUTC.Format(time.RFC3339),
		Price:        currentPrice,
		Timeframes:   tfDataMap,
		BTCContext:   ctxDataMap,
		Derivatives: DerivativesData{
			Funding:             funding,
			FundingHistory:      fundingHistoryList,
			OpenInterest:        currentOI,
			OIChange15m:         oiChange15m,
			OIChange1h:          oiChange1h,
			OIChange4h:          oiChange4h,
			OrderBookBidPercent: bidPercent,
			OrderBookAskPercent: askPercent,
		},
		BTCAgreementScore: agreementScore,
		AriDecision: AriDecision{
			LongConfidence:  nil,
			ShortConfidence: nil,
			WaitConfidence:  nil,
			Recommendation:  nil,
			Reasoning:       nil,
		},
		FutureOutcome: FutureOutcome{
			After30m:         nil,
			After1h:          nil,
			After4h:          nil,
			MaxFavorableMove: nil,
			MaxAdverseMove:   nil,
		},
	}

	// Write Snapshot JSON
	snapshotDir := "runs/scout/snapshots"
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		logError("Failed to create snapshot directory: %v", err)
		os.Exit(1)
	}

	filename := fmt.Sprintf("snapshot_%s_%s.json", nowUTC.Format("20060102_150405"), symbol)
	fullPath := filepath.Join(snapshotDir, filename)

	snapshotBytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		logError("Failed to marshal snapshot to JSON: %v", err)
		os.Exit(1)
	}

	if err := os.WriteFile(fullPath, snapshotBytes, 0644); err != nil {
		logError("Failed to write snapshot file: %v", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Saved snapshot to [snapshot](file://%s)\n\n", filepath.Clean(fullPath))

	// Print paste-ready report
	printPasteReadyReport(snapshot, fullPath, contextSymbol, ctxPrice)
}

func calculateTimeframeData(candles []Candle, currentPrice float64) TimeframeData {
	closes := getCloses(candles)
	highs := getHighs(candles)
	lows := getLows(candles)

	ma20 := SMA(closes, 20)
	ma100 := SMA(closes, 100)
	ma200 := SMA(closes, 200)

	distMA20 := 0.0
	if ma20 > 0 {
		distMA20 = ((currentPrice - ma20) / ma20) * 100
	}
	distMA100 := 0.0
	if ma100 > 0 {
		distMA100 = ((currentPrice - ma100) / ma100) * 100
	}
	distMA200 := 0.0
	if ma200 > 0 {
		distMA200 = ((currentPrice - ma200) / ma200) * 100
	}

	rsi6 := CalculateRSI(closes, 6)
	k, d, j := CalculateKDJ(candles, 9, 3, 3)

	atr := ATR(highs, lows, closes, 14)
	volRatio := calculateVolumeRatio(candles, 20)

	// Swing highs/lows based on highest high/lowest low in the last 20 candles
	swingHigh := 0.0
	swingLow := 0.0
	if len(candles) >= 21 {
		lookbackSlice := candles[len(candles)-21 : len(candles)-1] // Exclude current forming candle
		swingHigh = lookbackSlice[0].High
		swingLow = lookbackSlice[0].Low
		for _, c := range lookbackSlice {
			if c.High > swingHigh {
				swingHigh = c.High
			}
			if c.Low < swingLow {
				swingLow = c.Low
			}
		}
	}

	return TimeframeData{
		MA20:         ma20,
		MA100:        ma100,
		MA200:        ma200,
		DistanceMA20: distMA20,
		DistanceMA10: distMA100,
		DistanceMA200: distMA200,
		RSI6:         rsi6,
		KDJ:          []float64{k, d, j},
		ATR:          atr,
		VolumeRatio:  volRatio,
		SwingHigh:    swingHigh,
		SwingLow:     swingLow,
	}
}

func printPasteReadyReport(snapshot MarketSnapshot, path, contextSymbol string, contextPrice float64) {
	fmt.Println("================================================================================")
	fmt.Printf("=== AK SCOUT MARKET SNAPSHOT FOR %s ===\n", snapshot.Symbol)
	fmt.Printf("Price: %.4f | Context: %s (Price: %.2f)\n", snapshot.Price, contextSymbol, contextPrice)
	fmt.Printf("Timestamp (UTC): %s\n", snapshot.TimestampUTC)
	fmt.Printf("Snapshot File: %s\n", path)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Timeframe Indicators:")
	for _, tf := range []string{"15m", "30m", "1h", "4h", "1d"} {
		data := snapshot.Timeframes[tf]
		fmt.Printf("[%4s] MA20: %8.2f (Dist: %+5.2f%%) | MA100: %8.2f | MA200: %8.2f\n",
			tf, data.MA20, data.DistanceMA20, data.MA100, data.MA200)
		fmt.Printf("       RSI6: %5.2f | KDJ: K=%.2f D=%.2f J=%.2f | ATR: %5.2f | VolRatio: %4.2f\n",
			data.RSI6, data.KDJ[0], data.KDJ[1], data.KDJ[2], data.ATR, data.VolumeRatio)
		if data.SwingHigh > 0 {
			fmt.Printf("       Swing High: %8.2f | Swing Low: %8.2f\n", data.SwingHigh, data.SwingLow)
		}
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Context (BTC) Indicators:")
	for _, tf := range []string{"15m", "1h", "4h"} {
		data := snapshot.BTCContext[tf]
		fmt.Printf("[%4s] MA20: %8.2f (Dist: %+5.2f%%) | MA100: %8.2f | MA200: %8.2f\n",
			tf, data.MA20, data.DistanceMA20, data.MA100, data.MA200)
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Derivatives details:")
	d := snapshot.Derivatives
	fmt.Printf("Funding Rate: %f\n", d.Funding)
	if len(d.FundingHistory) > 0 {
		var histStrs []string
		for _, val := range d.FundingHistory {
			histStrs = append(histStrs, fmt.Sprintf("%f", val))
		}
		fmt.Printf("Funding History: [%s]\n", strings.Join(histStrs, ", "))
	}
	fmt.Printf("Open Interest: %.0f | 15m Chg: %+5.2f%% | 1h Chg: %+5.2f%% | 4h Chg: %+5.2f%%\n",
		d.OpenInterest, d.OIChange15m, d.OIChange1h, d.OIChange4h)
	fmt.Printf("Order Book Imbalance: Bid: %.2f%% | Ask: %.2f%%\n", d.OrderBookBidPercent, d.OrderBookAskPercent)
	fmt.Printf("BTC Agreement Score: %.1f%%\n", snapshot.BTCAgreementScore)
	fmt.Println("================================================================================")
	fmt.Println("Please analyze the market snapshot and copy/paste your decision using this template:")
	fmt.Println()
	fmt.Println("LONG: x/10")
	fmt.Println("SHORT: x/10")
	fmt.Println("WAIT: x/10")
	fmt.Println("Best action: <LONG/SHORT/WAIT>")
	fmt.Println("Invalidation: <Price or setup invalidation level>")
	fmt.Println("Targets: <TP1, TP2 etc>")
	fmt.Println("Lesson: <Any specific observation or rule learned>")
	fmt.Println("================================================================================")
}

func runRecord(args []string) {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	snapshotOpt := fs.String("snapshot", "", "Path to snapshot JSON file")
	recommendationOpt := fs.String("recommendation", "", "LONG, SHORT, or WAIT")
	longConfOpt := fs.Float64("long-conf", -1, "LONG confidence score (0-10)")
	shortConfOpt := fs.Float64("short-conf", -1, "SHORT confidence score (0-10)")
	waitConfOpt := fs.Float64("wait-conf", -1, "WAIT confidence score (0-10)")
	entryOpt := fs.Float64("entry", 0, "Optional entry price")
	stopOpt := fs.Float64("stop", 0, "Optional stop loss price")
	tp1Opt := fs.Float64("tp1", 0, "Optional take profit 1")
	tp2Opt := fs.Float64("tp2", 0, "Optional take profit 2")
	reasonOpt := fs.String("reason", "", "Optional reason description")
	fs.Parse(args)

	if *snapshotOpt == "" || *recommendationOpt == "" || *longConfOpt < 0 || *shortConfOpt < 0 || *waitConfOpt < 0 {
		logError("Error: Missing required arguments. Running with help:")
		fs.Usage()
		os.Exit(1)
	}

	rec := strings.ToUpper(*recommendationOpt)
	if rec != "LONG" && rec != "SHORT" && rec != "WAIT" {
		logError("Error: Recommendation must be LONG, SHORT, or WAIT")
		os.Exit(1)
	}

	// Check if snapshot JSON exists
	snapshotBytes, err := os.ReadFile(*snapshotOpt)
	if err != nil {
		logError("Failed to read snapshot file: %v", err)
		os.Exit(1)
	}

	var snapshot MarketSnapshot
	if err := json.Unmarshal(snapshotBytes, &snapshot); err != nil {
		logError("Failed to parse snapshot JSON: %v", err)
		os.Exit(1)
	}

	// Update snapshot object with decision
	snapshot.AriDecision = AriDecision{
		LongConfidence:  longConfOpt,
		ShortConfidence: shortConfOpt,
		WaitConfidence:  waitConfOpt,
		Recommendation:  &rec,
		Reasoning:       reasonOpt,
	}

	// Save updated snapshot JSON back
	updatedBytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		logError("Failed to marshal updated snapshot: %v", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*snapshotOpt, updatedBytes, 0644); err != nil {
		logError("Failed to save updated snapshot: %v", err)
		os.Exit(1)
	}

	// Append to runs/scout/journal.csv
	journalDir := "runs/scout"
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		logError("Failed to create runs/scout directory: %v", err)
		os.Exit(1)
	}

	journalPath := filepath.Join(journalDir, "journal.csv")
	fileExisted := true
	if _, err := os.Stat(journalPath); os.IsNotExist(err) {
		fileExisted = false
	}

	file, err := os.OpenFile(journalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logError("Failed to open journal file: %v", err)
		os.Exit(1)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if !fileExisted {
		headers := []string{
			"timestamp", "symbol", "price", "recommendation", "long_conf", "short_conf", "wait_conf",
			"entry", "stop", "tp1", "tp2", "reason", "snapshot_file", "after_30m", "after_1h", "after_4h",
			"max_favorable", "max_adverse", "outcome_status",
		}
		writer.Write(headers)
	}

	row := []string{
		snapshot.TimestampUTC,
		snapshot.Symbol,
		fmt.Sprintf("%.4f", snapshot.Price),
		rec,
		fmt.Sprintf("%.1f", *longConfOpt),
		fmt.Sprintf("%.1f", *shortConfOpt),
		fmt.Sprintf("%.1f", *waitConfOpt),
		fmt.Sprintf("%.4f", *entryOpt),
		fmt.Sprintf("%.4f", *stopOpt),
		fmt.Sprintf("%.4f", *tp1Opt),
		fmt.Sprintf("%.4f", *tp2Opt),
		*reasonOpt,
		*snapshotOpt,
		"", "", "", "", "", "PENDING",
	}

	if err := writer.Write(row); err != nil {
		logError("Failed to write row to CSV: %v", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Decision successfully recorded in journal.csv and snapshot JSON updated!\n")
}

func runCheckOutcomes(args []string) {
	journalPath := "runs/scout/journal.csv"
	file, err := os.Open(journalPath)
	if err != nil {
		logError("Failed to open journal file: %v", err)
		os.Exit(1)
	}

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	file.Close()
	if err != nil {
		logError("Failed to read journal file: %v", err)
		os.Exit(1)
	}

	if len(rows) <= 1 {
		fmt.Println("No records to check.")
		return
	}

	headers := rows[0]
	headerMap := make(map[string]int)
	for i, h := range headers {
		headerMap[h] = i
	}

	updatedCount := 0
	for idx := 1; idx < len(rows); idx++ {
		row := rows[idx]
		status := row[headerMap["outcome_status"]]
		if status != "PENDING" && status != "" {
			continue // Already processed
		}

		timestampStr := row[headerMap["timestamp"]]
		t, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			continue
		}

		// Check if 4 hours have passed
		if time.Since(t) < 4*time.Hour {
			continue // Not ready yet
		}

		symbol := row[headerMap["symbol"]]
		snapshotFile := row[headerMap["snapshot_file"]]
		entryPrice, _ := strconv.ParseFloat(row[headerMap["price"]], 64)
		rec := row[headerMap["recommendation"]]
		stopVal, _ := strconv.ParseFloat(row[headerMap["stop"]], 64)
		tp1Val, _ := strconv.ParseFloat(row[headerMap["tp1"]], 64)

		fmt.Printf("Checking outcomes for %s at %s...\n", symbol, timestampStr)

		// Fetch 1m candles starting at snapshot timestamp
		candles, err := fetchKlinesStartingAt(symbol, "1m", 240, t.UnixMilli())
		if err != nil || len(candles) < 240 {
			fmt.Printf("⚠️ Not enough historical 1m candles yet or fetch failed (fetched %d, need 240): %v\n", len(candles), err)
			continue
		}

		// Calculate values
		p30m := candles[29].Close
		p1h := candles[59].Close
		p4h := candles[239].Close

		maxHigh := candles[0].High
		minLow := candles[0].Low
		for i := 0; i < 240; i++ {
			if candles[i].High > maxHigh {
				maxHigh = candles[i].High
			}
			if candles[i].Low < minLow {
				minLow = candles[i].Low
			}
		}

		// Calculate moves
		var maxFavorable, maxAdverse float64
		if rec == "LONG" {
			maxFavorable = ((maxHigh - entryPrice) / entryPrice) * 100
			maxAdverse = ((entryPrice - minLow) / entryPrice) * 100
		} else if rec == "SHORT" {
			maxFavorable = ((entryPrice - minLow) / entryPrice) * 100
			maxAdverse = ((maxHigh - entryPrice) / entryPrice) * 100
		} else { // WAIT
			maxFavorable = math.Max(maxHigh-entryPrice, entryPrice-minLow) / entryPrice * 100
			maxAdverse = math.Min(maxHigh-entryPrice, entryPrice-minLow) / entryPrice * 100
		}

		// Check if stop loss or take profit was hit first
		outcomeStatus := "CHOP"
		if rec == "LONG" && stopVal > 0 && tp1Val > 0 {
			for i := 0; i < 240; i++ {
				if candles[i].Low <= stopVal {
					outcomeStatus = "STOPPED"
					break
				}
				if candles[i].High >= tp1Val {
					outcomeStatus = "TARGET_HIT"
					break
				}
			}
		} else if rec == "SHORT" && stopVal > 0 && tp1Val > 0 {
			for i := 0; i < 240; i++ {
				if candles[i].High >= stopVal {
					outcomeStatus = "STOPPED"
					break
				}
				if candles[i].Low <= tp1Val {
					outcomeStatus = "TARGET_HIT"
					break
				}
			}
		} else if rec == "WAIT" {
			// A wait is correct if it did not trend significantly (max move less than 1.5%)
			if maxHigh/entryPrice < 1.015 && minLow/entryPrice > 0.985 {
				outcomeStatus = "CORRECT_WAIT"
			} else {
				outcomeStatus = "MISSED_TREND"
			}
		}

		// Update Row in Memory
		row[headerMap["after_30m"]] = fmt.Sprintf("%.4f", p30m)
		row[headerMap["after_1h"]] = fmt.Sprintf("%.4f", p1h)
		row[headerMap["after_4h"]] = fmt.Sprintf("%.4f", p4h)
		row[headerMap["max_favorable"]] = fmt.Sprintf("%.2f%%", maxFavorable)
		row[headerMap["max_adverse"]] = fmt.Sprintf("%.2f%%", maxAdverse)
		row[headerMap["outcome_status"]] = outcomeStatus

		// Update Snapshot JSON
		if snapshotFile != "" {
			updateSnapshotFileOutcome(snapshotFile, p30m, p1h, p4h, maxFavorable, maxAdverse)
		}

		updatedCount++
	}

	if updatedCount > 0 {
		// Save journal back
		outFile, err := os.Create(journalPath)
		if err != nil {
			logError("Failed to save journal file: %v", err)
			os.Exit(1)
		}
		defer outFile.Close()

		writer := csv.NewWriter(outFile)
		writer.WriteAll(rows)
		writer.Flush()
		fmt.Printf("🎉 Successfully updated %d journal records!\n", updatedCount)
	} else {
		fmt.Println("No outcomes were due or needed update.")
	}
}

func updateSnapshotFileOutcome(filePath string, p30m, p1h, p4h, maxFav, maxAdv float64) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	var s MarketSnapshot
	if err := json.Unmarshal(bytes, &s); err != nil {
		return
	}

	s.FutureOutcome = FutureOutcome{
		After30m:         &p30m,
		After1h:          &p1h,
		After4h:          &p4h,
		MaxFavorableMove: &maxFav,
		MaxAdverseMove:   &maxAdv,
	}

	updated, err := json.MarshalIndent(s, "", "  ")
	if err == nil {
		os.WriteFile(filePath, updated, 0644)
	}
}

func runReport(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	fromOpt := fs.String("from", "", "Start date (YYYY-MM-DD)")
	toOpt := fs.String("to", "", "End date (YYYY-MM-DD)")
	fs.Parse(args)

	journalPath := "runs/scout/journal.csv"
	file, err := os.Open(journalPath)
	if err != nil {
		logError("Failed to open journal: %v", err)
		os.Exit(1)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		logError("Failed to read journal: %v", err)
		os.Exit(1)
	}

	if len(rows) <= 1 {
		fmt.Println("No data available in decision journal.")
		return
	}

	headers := rows[0]
	headerMap := make(map[string]int)
	for i, h := range headers {
		headerMap[h] = i
	}

	var fromTime, toTime time.Time
	if *fromOpt != "" {
		fromTime, _ = time.Parse("2006-01-02", *fromOpt)
	}
	if *toOpt != "" {
		toTime, _ = time.Parse("2006-01-02", *toOpt)
	}

	totalSnapshots := 0
	totalWait := 0
	correctWait := 0

	totalLong := 0
	profitableLong := 0
	stoppedLong := 0
	chopLong := 0

	totalShort := 0
	profitableShort := 0
	stoppedShort := 0
	chopShort := 0

	for idx := 1; idx < len(rows); idx++ {
		row := rows[idx]
		timestampStr := row[headerMap["timestamp"]]
		t, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			continue
		}

		if !fromTime.IsZero() && t.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && t.After(toTime.Add(24*time.Hour)) {
			continue
		}

		totalSnapshots++
		rec := row[headerMap["recommendation"]]
		status := row[headerMap["outcome_status"]]

		switch rec {
		case "WAIT":
			totalWait++
			if status == "CORRECT_WAIT" {
				correctWait++
			}
		case "LONG":
			totalLong++
			if status == "TARGET_HIT" {
				profitableLong++
			} else if status == "STOPPED" {
				stoppedLong++
			} else if status == "CHOP" || status == "MISSED_TREND" {
				chopLong++
			}
		case "SHORT":
			totalShort++
			if status == "TARGET_HIT" {
				profitableShort++
			} else if status == "STOPPED" {
				stoppedShort++
			} else if status == "CHOP" || status == "MISSED_TREND" {
				chopShort++
			}
		}
	}

	fmt.Println("================================================================================")
	fmt.Println("=== ARI CONFIDENCE CALIBRATION REPORT ===")
	if *fromOpt != "" || *toOpt != "" {
		fmt.Printf("Period: %s to %s\n", *fromOpt, *toOpt)
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("Total Snapshots Rated: %d\n\n", totalSnapshots)

	fmt.Printf("WAIT Recommendations: %d\n", totalWait)
	if totalWait > 0 {
		fmt.Printf("  - Correct WAITs (Protected range-bound): %d (%.1f%%)\n", correctWait, (float64(correctWait)/float64(totalWait))*100)
	} else {
		fmt.Println("  - No WAIT recommendations recorded.")
	}
	fmt.Println()

	fmt.Printf("LONG Recommendations: %d\n", totalLong)
	if totalLong > 0 {
		fmt.Printf("  - Profitable (Target Hit): %d (%.1f%%)\n", profitableLong, (float64(profitableLong)/float64(totalLong))*100)
		fmt.Printf("  - Stopped First:           %d (%.1f%%)\n", stoppedLong, (float64(stoppedLong)/float64(totalLong))*100)
		fmt.Printf("  - Chop/No Edge:            %d (%.1f%%)\n", chopLong, (float64(chopLong)/float64(totalLong))*100)
	} else {
		fmt.Println("  - No LONG recommendations recorded.")
	}
	fmt.Println()

	fmt.Printf("SHORT Recommendations: %d\n", totalShort)
	if totalShort > 0 {
		fmt.Printf("  - Profitable (Target Hit): %d (%.1f%%)\n", profitableShort, (float64(profitableShort)/float64(totalShort))*100)
		fmt.Printf("  - Stopped First:           %d (%.1f%%)\n", stoppedShort, (float64(stoppedShort)/float64(totalShort))*100)
		fmt.Printf("  - Chop/No Edge:            %d (%.1f%%)\n", chopShort, (float64(chopShort)/float64(totalShort))*100)
	} else {
		fmt.Println("  - No SHORT recommendations recorded.")
	}
	fmt.Println("================================================================================")
}

// --- REST Client Functions ---

func fetchPremiumIndex(symbol string) (*PremiumIndexItem, error) {
	url := fmt.Sprintf("https://dapi.binance.com/dapi/v1/premiumIndex?symbol=%s", symbol)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	var items []PremiumIndexItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no premium index returned")
	}

	return &items[0], nil
}

type FundingHistoryItem struct {
	Symbol      string `json:"symbol"`
	FundingTime int64  `json:"fundingTime"`
	FundingRate string `json:"fundingRate"`
}

func fetchFundingHistory(symbol string, limit int) ([]FundingHistoryItem, error) {
	url := fmt.Sprintf("https://dapi.binance.com/dapi/v1/fundingRate?symbol=%s&limit=%d", symbol, limit)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	var items []FundingHistoryItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	return items, nil
}

func fetchOpenInterest(symbol string) (*OpenInterestItem, error) {
	url := fmt.Sprintf("https://dapi.binance.com/dapi/v1/openInterest?symbol=%s", symbol)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	var item OpenInterestItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}
	return &item, nil
}

func fetchOpenInterestHistory(pair, period string, limit int) ([]OpenInterestHistItem, error) {
	url := fmt.Sprintf("https://dapi.binance.com/futures/data/openInterestHist?pair=%s&contractType=PERPETUAL&period=%s&limit=%d", pair, period, limit)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	var items []OpenInterestHistItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func fetchOrderBook(symbol string, limit int) (*DepthItem, error) {
	url := fmt.Sprintf("https://dapi.binance.com/dapi/v1/depth?symbol=%s&limit=%d", symbol, limit)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	var item DepthItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}
	return &item, nil
}

func fetchKlines(symbol, interval string, limit int) ([]Candle, error) {
	url := fmt.Sprintf("https://dapi.binance.com/dapi/v1/klines?symbol=%s&interval=%s&limit=%d", symbol, interval, limit)
	return fetchKlinesFromURL(url, symbol, interval)
}

func fetchKlinesStartingAt(symbol, interval string, limit int, startTime int64) ([]Candle, error) {
	url := fmt.Sprintf("https://dapi.binance.com/dapi/v1/klines?symbol=%s&interval=%s&limit=%d&startTime=%d", symbol, interval, limit, startTime)
	return fetchKlinesFromURL(url, symbol, interval)
}

func fetchKlinesFromURL(url, symbol, interval string) ([]Candle, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	var rawKlines [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawKlines); err != nil {
		return nil, err
	}

	var candles []Candle
	for _, k := range rawKlines {
		if len(k) < 8 {
			continue
		}

		openTime := int64(k[0].(float64))
		open, _ := strconv.ParseFloat(k[1].(string), 64)
		high, _ := strconv.ParseFloat(k[2].(string), 64)
		low, _ := strconv.ParseFloat(k[3].(string), 64)
		closeP, _ := strconv.ParseFloat(k[4].(string), 64)
		vol, _ := strconv.ParseFloat(k[5].(string), 64)
		closeTime := int64(k[6].(float64))
		quoteVol, _ := strconv.ParseFloat(k[7].(string), 64)

		candles = append(candles, Candle{
			Symbol:           symbol,
			Interval:         interval,
			OpenTime:         openTime,
			Open:             open,
			High:             high,
			Low:              low,
			Close:            closeP,
			Volume:           vol,
			CloseTime:        closeTime,
			QuoteAssetVolume: quoteVol,
		})
	}

	return candles, nil
}

func fetchContextPrice(contextSymbol string) (float64, error) {
	prem, err := fetchPremiumIndex(contextSymbol)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(prem.MarkPrice, 64)
}

// --- Math & Indicator Helpers ---

func SMA(data []float64, period int) float64 {
	if len(data) < period {
		return 0
	}
	var sum float64
	for i := len(data) - period; i < len(data); i++ {
		sum += data[i]
	}
	return sum / float64(period)
}

func CalculateRSI(closes []float64, period int) float64 {
	if len(closes) <= period {
		return 50.0
	}
	var gains, losses float64
	for i := 1; i <= period; i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	for i := period + 1; i < len(closes); i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			avgGain = (avgGain*float64(period-1) + diff) / float64(period)
			avgLoss = (avgLoss*float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain*float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) - diff) / float64(period)
		}
	}
	if avgLoss == 0 {
		return 100.0
	}
	rs := avgGain / avgLoss
	return 100.0 - (100.0 / (1.0 + rs))
}

func CalculateKDJ(candles []Candle, n, m1, m2 int) (float64, float64, float64) {
	if len(candles) < n {
		return 50.0, 50.0, 50.0
	}
	k := 50.0
	d := 50.0
	var j float64

	for i := n - 1; i < len(candles); i++ {
		highN := candles[i].High
		lowN := candles[i].Low
		for idx := i - n + 1; idx <= i; idx++ {
			if candles[idx].High > highN {
				highN = candles[idx].High
			}
			if candles[idx].Low < lowN {
				lowN = candles[idx].Low
			}
		}
		denominator := highN - lowN
		rsv := 50.0
		if denominator > 0 {
			rsv = ((candles[i].Close - lowN) / denominator) * 100.0
		}
		k = (float64(m1-1)/float64(m1))*k + (1.0/float64(m1))*rsv
		d = (float64(m2-1)/float64(m2))*d + (1.0/float64(m2))*k
		j = 3.0*k - 2.0*d
	}
	return k, d, j
}

func calculateVolumeRatio(candles []Candle, period int) float64 {
	if len(candles) < period+1 {
		return 1.0
	}
	currentVol := candles[len(candles)-1].Volume
	var sum float64
	for i := len(candles) - period - 1; i < len(candles)-1; i++ {
		sum += candles[i].Volume
	}
	avgVol := sum / float64(period)
	if avgVol == 0 {
		return 1.0
	}
	return currentVol / avgVol
}

func calculateOrderBookImbalance(depth *DepthItem) (bidPercent, askPercent float64) {
	var totalBid, totalAsk float64
	for _, bid := range depth.Bids {
		qty, _ := strconv.ParseFloat(bid[1], 64)
		totalBid += qty
	}
	for _, ask := range depth.Asks {
		qty, _ := strconv.ParseFloat(ask[1], 64)
		totalAsk += qty
	}

	total := totalBid + totalAsk
	if total == 0 {
		return 50.0, 50.0
	}
	return (totalBid / total) * 100.0, (totalAsk / total) * 100.0
}

func calculateAgreementScore(symbolMap, btcMap map[string]TimeframeData) float64 {
	timeframes := []string{"15m", "1h", "4h"}
	matches := 0
	for _, tf := range timeframes {
		sym := symbolMap[tf]
		btc := btcMap[tf]

		// Relation to MA20: Close is currentPrice, distance is currentPrice vs MA20.
		// Positive distance means Close > MA20, negative means Close < MA20.
		symBull := sym.DistanceMA20 > 0
		btcBull := btc.DistanceMA20 > 0

		if symBull == btcBull {
			matches++
		}
	}
	return (float64(matches) / 3.0) * 100.0
}

func getCloses(candles []Candle) []float64 {
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	return closes
}

func getHighs(candles []Candle) []float64 {
	highs := make([]float64, len(candles))
	for i, c := range candles {
		highs[i] = c.High
	}
	return highs
}

func getLows(candles []Candle) []float64 {
	lows := make([]float64, len(candles))
	for i, c := range candles {
		lows[i] = c.Low
	}
	return lows
}

func logError(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func ATR(high, low, close []float64, period int) float64 {
	if len(close) <= period {
		return 0
	}
	var trSum float64
	for i := 1; i <= period; i++ {
		trSum += math.Max(high[i]-low[i], math.Max(math.Abs(high[i]-close[i-1]), math.Abs(low[i]-close[i-1])))
	}
	atr := trSum / float64(period)
	for i := period + 1; i < len(close); i++ {
		tr := math.Max(high[i]-low[i], math.Max(math.Abs(high[i]-close[i-1]), math.Abs(low[i]-close[i-1])))
		atr = (atr*float64(period-1) + tr) / float64(period)
	}
	return atr
}
