package applicationrecommendation

import (
	"GCFeed/internal/application/pagination"
	domainembedding "GCFeed/internal/domain/embedding"
	domainrecommendation "GCFeed/internal/domain/recommendation"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sync/singleflight"
	"math"
	"sort"
	"strings"
	"time"
)

const defaultLimit = 10
const candidatePoolMultiplier = 8
const minCandidatePoolSize = 50
const maxCandidatePoolSize = 500

var ErrLoadRecommendationFailed = errors.New("failed to load recommendations")
var ErrLoadExposureDecisionsFailed = errors.New("failed to load exposure decisions")
var ErrSaveRecommendationExposureFailed = errors.New("failed to save recommendation exposure")

type Service struct {
	repo          domainrecommendation.Repository
	now           func() time.Time
	snapshots     pagination.Cache
	interests     pagination.Cache
	interestGroup singleflight.Group
}

type Option func(*Service)

type CandidateRequest struct {
	UserID    int64
	Scene     string
	RequestID string
	Cursor    string
	Limit     int
}

type CandidateResult struct {
	UserID     int64
	Scene      string
	RequestID  string
	Candidates []*domainrecommendation.Candidate
	NextCursor string
	HasMore    bool
}

type ExposureInput struct {
	UserID    int64
	VideoID   int64
	Scene     string
	RequestID string
}

type ExposureDecisionInput struct {
	UserID    int64
	Scene     string
	RequestID string
	VideoIDs  []int64
}

type ExposureDecisionResult struct {
	UserID    int64
	Scene     string
	RequestID string
	Decisions []*domainrecommendation.ExposureDecision
}

type ExposureResult struct {
	Exposures []*domainrecommendation.Exposure
}

func New(repo domainrecommendation.Repository, options ...Option) *Service {
	service := &Service{
		repo:      repo,
		snapshots: pagination.NewMemoryCache(256),
		interests: pagination.NewMemoryCache(1024),
		now:       func() time.Time { return time.Now().UTC().Truncate(time.Minute) },
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithSnapshotStore enables shared bounded snapshots and interest caching across API replicas.
func WithSnapshotStore(cache pagination.Cache) Option {
	return func(s *Service) {
		if cache != nil {
			s.snapshots = cache
			s.interests = cache
		}
	}
}

func (s *Service) Recommend(ctx context.Context, input CandidateRequest) (*CandidateResult, error) {
	limit := normalizeLimit(input.Limit)
	snapshotCursor, err := pagination.Decode(strings.TrimSpace(input.Cursor))
	if err != nil {
		return nil, domainrecommendation.ErrInvalidCursor
	}
	req, err := domainrecommendation.NewCandidateRequest(input.UserID, input.Scene, input.RequestID, limit)
	if err != nil {
		return nil, err
	}
	var snapshot rankedSnapshot
	id, offset := "", 0
	if snapshotCursor != nil {
		id, offset = snapshotCursor.ID, snapshotCursor.Offset
		content, found, err := s.snapshots.ReadBlob(ctx, "recommend:"+id)
		if err != nil {
			return nil, ErrLoadRecommendationFailed
		}
		if !found || json.Unmarshal(content, &snapshot) != nil || snapshot.UserID != req.UserID || snapshot.Scene != req.Scene || offset > len(snapshot.Candidates) {
			return nil, domainrecommendation.ErrInvalidCursor
		}
	} else {
		pool, err := s.repo.ListCandidatePool(ctx, req.UserID, candidatePoolLimit(limit))
		if err != nil {
			return nil, ErrLoadRecommendationFailed
		}
		ranked, err := s.rankCandidates(ctx, req.UserID, pool)
		if err != nil {
			return nil, ErrLoadRecommendationFailed
		}
		ranked = interleaveByAuthor(ranked)
		if len(ranked) > pagination.MaxItems {
			ranked = ranked[:pagination.MaxItems]
		}
		snapshot = rankedSnapshot{UserID: req.UserID, Scene: req.Scene, Candidates: ranked}
		if len(ranked) > 0 {
			id, err = pagination.NewID()
			if err != nil {
				return nil, ErrLoadRecommendationFailed
			}
			content, err := json.Marshal(snapshot)
			if err != nil || s.snapshots.WriteBlob(ctx, "recommend:"+id, content, pagination.SnapshotTTL) != nil {
				return nil, ErrLoadRecommendationFailed
			}
		}
	}
	end := offset + limit
	if end > len(snapshot.Candidates) {
		end = len(snapshot.Candidates)
	}
	items := snapshot.Candidates[offset:end]
	if items == nil {
		items = []*domainrecommendation.Candidate{}
	}
	next := ""
	if id != "" && len(items) > 0 {
		next = pagination.Encode(id, end)
	}
	return &CandidateResult{UserID: req.UserID, Scene: req.Scene, RequestID: req.RequestID, Candidates: items, NextCursor: next, HasMore: end < len(snapshot.Candidates)}, nil
}

type rankedSnapshot struct {
	UserID     int64
	Scene      string
	Candidates []*domainrecommendation.Candidate
}

func (s *Service) DecideExposures(ctx context.Context, input ExposureDecisionInput) (*ExposureDecisionResult, error) {
	req, err := domainrecommendation.NewExposureDecisionRequest(input.UserID, input.Scene, input.RequestID, input.VideoIDs)
	if err != nil {
		return nil, err
	}
	if len(req.VideoIDs) == 0 {
		return &ExposureDecisionResult{
			UserID:    req.UserID,
			Scene:     req.Scene,
			RequestID: req.RequestID,
			Decisions: []*domainrecommendation.ExposureDecision{},
		}, nil
	}

	exposures, err := s.repo.ListRecentExposures(ctx, req.UserID, req.VideoIDs, s.now().Add(-domainrecommendation.RecentExposureWindow))
	if err != nil {
		return nil, ErrLoadExposureDecisionsFailed
	}
	exposureByVideoID := make(map[int64]*domainrecommendation.Exposure, len(exposures))
	for _, exposure := range exposures {
		if exposure != nil {
			exposureByVideoID[exposure.VideoID] = exposure
		}
	}

	decisions := make([]*domainrecommendation.ExposureDecision, 0, len(req.VideoIDs))
	for _, videoID := range req.VideoIDs {
		if exposure := exposureByVideoID[videoID]; exposure != nil {
			decisions = append(decisions, domainrecommendation.RestoreExposureDecision(
				videoID,
				false,
				domainrecommendation.ExposureDecisionReasonRecentlyExposed,
				&exposure.LastExposedAt,
			))
			continue
		}
		decisions = append(decisions, domainrecommendation.RestoreExposureDecision(
			videoID,
			true,
			domainrecommendation.ExposureDecisionReasonFresh,
			nil,
		))
	}
	return &ExposureDecisionResult{
		UserID:    req.UserID,
		Scene:     req.Scene,
		RequestID: req.RequestID,
		Decisions: decisions,
	}, nil
}

func (s *Service) SaveExposures(ctx context.Context, inputs []ExposureInput) (*ExposureResult, error) {
	writes := make([]*domainrecommendation.ExposureWrite, 0, len(inputs))
	seen := map[int64]struct{}{}
	for _, input := range inputs {
		write, err := domainrecommendation.NewExposureWrite(input.UserID, input.VideoID, input.Scene, input.RequestID)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[write.VideoID]; exists {
			continue
		}
		seen[write.VideoID] = struct{}{}
		writes = append(writes, write)
	}
	if len(writes) == 0 {
		return &ExposureResult{Exposures: []*domainrecommendation.Exposure{}}, nil
	}

	exposures, err := s.repo.SaveExposures(ctx, writes)
	if err != nil {
		if errors.Is(err, domainrecommendation.ErrVideoNotFound) {
			return nil, err
		}
		return nil, ErrSaveRecommendationExposureFailed
	}
	return &ExposureResult{Exposures: exposures}, nil
}

func (s *Service) rankCandidates(ctx context.Context, userID int64, pool []*domainrecommendation.Candidate) ([]*domainrecommendation.Candidate, error) {
	if len(pool) == 0 {
		return []*domainrecommendation.Candidate{}, nil
	}

	videoIDs := make([]int64, 0, len(pool))
	for _, candidate := range pool {
		if candidate != nil && candidate.VideoID > 0 {
			videoIDs = append(videoIDs, candidate.VideoID)
		}
	}
	vectors, err := s.repo.LoadVideoVectors(ctx, videoIDs)
	if err != nil {
		return nil, err
	}
	userVector, hasUserVector, err := s.loadInterest(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	ranked := make([]*domainrecommendation.Candidate, 0, len(pool))
	for _, candidate := range pool {
		if candidate == nil {
			continue
		}
		value := *candidate
		value.FreshnessScore = freshnessScore(now, value.PublishedAt)
		value.Similarity = 0
		if hasUserVector {
			if vector := vectors[value.VideoID]; len(vector) > 0 {
				similarity, err := domainembedding.CosineSimilarity(userVector, vector)
				if err == nil {
					value.Similarity = similarity
				}
			}
		}
		value.RankScore = rankScore(value.Similarity, value.HotScore, value.FreshnessScore, hasUserVector)
		value.Reason = recommendationReason(hasUserVector, value.Similarity, value.HotScore)
		ranked = append(ranked, &value)
	}

	sortCandidates(ranked)
	return ranked, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > domainrecommendation.MaxLimit {
		return domainrecommendation.MaxLimit
	}
	return limit
}

func candidatePoolLimit(limit int) int {
	poolLimit := limit * candidatePoolMultiplier
	if poolLimit < minCandidatePoolSize {
		poolLimit = minCandidatePoolSize
	}
	if poolLimit > maxCandidatePoolSize {
		poolLimit = maxCandidatePoolSize
	}
	return poolLimit
}

func rankScore(similarity float64, hotScore int, freshness float64, hasUserVector bool) float64 {
	hot := math.Log1p(float64(maxInt(hotScore, 0))) / 10
	if hasUserVector {
		return similarity*0.70 + hot*0.20 + freshness*0.10
	}
	return hot*0.65 + freshness*0.35
}

func freshnessScore(now time.Time, publishedAt time.Time) float64 {
	if publishedAt.IsZero() {
		return 0
	}
	hours := now.Sub(publishedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return 1 / (1 + hours/72)
}

func recommendationReason(hasUserVector bool, similarity float64, hotScore int) string {
	if hasUserVector && similarity > 0.05 {
		return "interest_match"
	}
	if hotScore > 0 {
		return "hot"
	}
	return "fresh"
}

func sortCandidates(candidates []*domainrecommendation.Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.RankScore != right.RankScore {
			return left.RankScore > right.RankScore
		}
		if !left.PublishedAt.Equal(right.PublishedAt) {
			return left.PublishedAt.After(right.PublishedAt)
		}
		return left.VideoID > right.VideoID
	})
}

func interleaveByAuthor(candidates []*domainrecommendation.Candidate) []*domainrecommendation.Candidate {
	if len(candidates) <= 2 {
		return candidates
	}
	output := make([]*domainrecommendation.Candidate, 0, len(candidates))
	delayed := make([]*domainrecommendation.Candidate, 0)
	var previousAuthorID int64
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if previousAuthorID != 0 && candidate.AuthorID == previousAuthorID {
			delayed = append(delayed, candidate)
			continue
		}
		output = append(output, candidate)
		previousAuthorID = candidate.AuthorID
	}
	for len(delayed) > 0 {
		progressed := false
		remaining := make([]*domainrecommendation.Candidate, 0)
		for _, candidate := range delayed {
			if len(output) > 0 && output[len(output)-1].AuthorID == candidate.AuthorID {
				remaining = append(remaining, candidate)
				continue
			}
			output = append(output, candidate)
			progressed = true
		}
		if !progressed {
			output = append(output, remaining...)
			break
		}
		delayed = remaining
	}
	return output
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
