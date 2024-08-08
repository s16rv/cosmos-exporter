package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/types/query"
	govv1types "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

const LIMIT_PROPOSALS = 20

func ProposalsHandler(w http.ResponseWriter, r *http.Request, grpcConn *grpc.ClientConn) {
	requestStart := time.Now()

	sublogger := log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	proposalsGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_proposals",
			Help:        "Proposals of Cosmos-based blockchain",
			ConstLabels: ConstLabels,
		},
		[]string{"title", "status", "voting_start_time", "voting_end_time"},
	)

	registry := prometheus.NewRegistry()
	registry.MustRegister(proposalsGauge)

	var proposals []*govv1types.Proposal

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying proposals")
		queryStart := time.Now()

		govClient := govv1types.NewQueryClient(grpcConn)
		proposalsResponse, err := govClient.Proposals(
			context.Background(),
			&govv1types.QueryProposalsRequest{
				Pagination: &query.PageRequest{
					Limit:   LIMIT_PROPOSALS,
					Reverse: true,
				},
			},
		)
		if err != nil {
			sublogger.Error().Err(err).Msg("Could not get proposals")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying proposals")
		proposals = proposalsResponse.Proposals
	}()

	wg.Wait()

	sublogger.Debug().
		Int("proposalsLength", len(proposals)).
		Msg("Proposals info")

	for _, proposal := range proposals {
		proposalsGauge.With(prometheus.Labels{
			"title":             proposal.Title,
			"status":            proposal.Status.String(),
			"voting_start_time": proposal.VotingStartTime.String(),
			"voting_end_time":   proposal.VotingEndTime.String(),
		}).Set(float64(proposal.Id))
	}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/proposals").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
