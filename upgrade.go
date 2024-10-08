package main

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

func UpgradeHandler(w http.ResponseWriter, r *http.Request, grpcConn *grpc.ClientConn) {
	requestStart := time.Now()

	sublogger := log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	upgradePlanGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_upgrade_plan",
			Help:        "Upgrade plan info in height",
			ConstLabels: ConstLabels,
		},
		[]string{"info", "name", "time", "height", "estimated_time"},
	)

	registry := prometheus.NewRegistry()
	registry.MustRegister(upgradePlanGauge)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		queryStart := time.Now()

		upgradeClient := upgradetypes.NewQueryClient(grpcConn)
		upgradeRes, err := upgradeClient.CurrentPlan(
			context.Background(),
			&upgradetypes.QueryCurrentPlanRequest{},
		)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get upgrade plan")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying upgrade plan")

		if upgradeRes.Plan == nil {
			upgradePlanGauge.With(prometheus.Labels{
				"info":           "None",
				"name":           "None",
				"time":           "",
				"height":         "",
				"estimated_time": "",
			}).Set(0)
			return
		}

		cs, err := NewChainStatus()
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get sync info")
			return
		}

		upgradeHeight := upgradeRes.Plan.Height
		remainingHeight := upgradeHeight - cs.LatestBlockHeight()

		if remainingHeight <= 0 {
			upgradePlanGauge.With(prometheus.Labels{
				"info":           "None",
				"name":           "None",
				"time":           "",
				"height":         "",
				"estimated_time": "",
			}).Set(0)
			return
		}

		estimatedTime, err := cs.EstimateBlockTime(remainingHeight)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get estimated time")
		}

		upgradePlanGauge.With(prometheus.Labels{
			"info":           upgradeRes.Plan.Info,
			"name":           upgradeRes.Plan.Name,
			"time":           upgradeRes.Plan.Time.String(),
			"height":         strconv.FormatInt(upgradeHeight, 10),
			"estimated_time": estimatedTime.Local().Format(time.RFC1123),
		}).Set(float64(remainingHeight))
	}()

	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/upgrade").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
