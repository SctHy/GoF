package applicationrecommendation

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

const interestTTL = 30 * time.Second

type cachedInterest struct {
	Vector []float64
	Found  bool
}

func interestKey(userID int64) string { return "interest:v1:" + strconv.FormatInt(userID, 10) }

// InvalidateInterest is called after a committed viewing event. Existing ranked
// sessions remain immutable; fresh sessions reload the user's interest vector.
// Concurrent loads or external writers may retain the previous vector for at most TTL.
func (s *Service) InvalidateInterest(ctx context.Context, userID int64) error {
	s.interestGroup.Forget(interestKey(userID))
	return s.interests.DeleteBlob(ctx, interestKey(userID))
}

func (s *Service) loadInterest(ctx context.Context, userID int64) ([]float64, bool, error) {
	key := interestKey(userID)
	loadCached := func() (cachedInterest, bool) {
		content, found, err := s.interests.ReadBlob(ctx, key)
		if err != nil || !found {
			return cachedInterest{}, false
		}
		var interest cachedInterest
		if json.Unmarshal(content, &interest) != nil {
			return cachedInterest{}, false
		}
		return interest, true
	}
	if interest, ok := loadCached(); ok {
		return interest.Vector, interest.Found, nil
	}
	value, err, _ := s.interestGroup.Do(key, func() (any, error) {
		if interest, ok := loadCached(); ok {
			return interest, nil
		}
		vector, found, err := s.repo.LoadUserInterestVector(ctx, userID)
		if err != nil {
			return nil, err
		}
		interest := cachedInterest{Vector: vector, Found: found}
		if content, err := json.Marshal(interest); err == nil {
			_ = s.interests.WriteBlob(ctx, key, content, interestTTL)
		}
		return interest, nil
	})
	if err != nil {
		return nil, false, err
	}
	interest := value.(cachedInterest)
	return append([]float64(nil), interest.Vector...), interest.Found, nil
}
