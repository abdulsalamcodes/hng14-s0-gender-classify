package services

import (
	"context"
	"time"

	"hng14-s0-gender-classify/internal/models"
	"hng14-s0-gender-classify/internal/repository"
	"hng14-s0-gender-classify/pkg/api"

	"golang.org/x/sync/errgroup"
)

type Service struct {
	repo  *repository.Repository
	api   *api.Client
}

func New(repo *repository.Repository, apiClient *api.Client) *Service {
	return &Service{
		repo: repo,
		api:  apiClient,
	}
}

func (s *Service) ClassifyName(ctx context.Context, name string) (*models.DataPayload, error) {
	result, err := s.api.FetchGender(name)
	if err != nil {
		return nil, err
	}

	if result.Gender == "" || result.Count == 0 {
		return nil, ErrNoPrediction
	}

	isConfident := result.Probability >= 0.7 && result.Count >= 100

	return &models.DataPayload{
		Name:        result.Name,
		Gender:      result.Gender,
		Probability: result.Probability,
		SampleSize:  result.Count,
		IsConfident: isConfident,
		ProcessedAt: time.Now().UTC(),
	}, nil
}

func (s *Service) CreateProfile(ctx context.Context, name string) (*models.ProfilePayload, error) {
	existing, err := s.repo.GetProfileByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	genderResult, ageResult, natResult, err := s.fetchAllPredictions(ctx, name)
	if err != nil {
		return nil, err
	}

	if genderResult.Gender == "" || genderResult.Count == 0 {
		return nil, ErrNoGenderPrediction
	}
	if ageResult.Age == 0 {
		return nil, ErrNoAgePrediction
	}
	if len(natResult.Country) == 0 {
		return nil, ErrNoCountryPrediction
	}

	topCountry := natResult.Country[0]
	for _, c := range natResult.Country[1:] {
		if c.Probability > topCountry.Probability {
			topCountry = c
		}
	}

	profile := &models.ProfilePayload{
		Name:                genderResult.Name,
		Gender:              genderResult.Gender,
		GenderProbability:   genderResult.Probability,
		SampleSize:          genderResult.Count,
		Age:                 ageResult.Age,
		AgeGroup:            repository.ComputeAgeGroup(ageResult.Age),
		CountryID:           topCountry.CountryID,
		CountryName:         getCountryName(topCountry.CountryID),
		CountryProbability:  topCountry.Probability,
	}

	return s.repo.CreateProfile(ctx, profile)
}

func (s *Service) GetProfile(ctx context.Context, id string) (*models.ProfilePayload, error) {
	return s.repo.GetProfileByID(ctx, id)
}

func (s *Service) ListProfiles(ctx context.Context, filter repository.ProfileFilter) (*repository.PaginatedResult, error) {
	return s.repo.ListProfiles(ctx, filter)
}

func (s *Service) DeleteProfile(ctx context.Context, id string) (bool, error) {
	return s.repo.DeleteProfile(ctx, id)
}

func (s *Service) SeedProfile(ctx context.Context, p *models.SeedProfile) error {
	return s.repo.SeedProfile(ctx, p)
}

// GetUser loads a user by their internal UUID.
// Used by the /api/whoami handler so it can return full user details
// beyond what's embedded in the JWT claims (e.g. email, avatar_url).
func (s *Service) GetUser(ctx context.Context, id string) (*models.User, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *Service) fetchAllPredictions(ctx context.Context, name string) (*models.GenderResponse, *models.AgeResponse, *models.NationalizeResponse, error) {
	var genderResult models.GenderResponse
	var ageResult models.AgeResponse
	var natResult models.NationalizeResponse

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		result, err := s.api.FetchGender(name)
		if err != nil {
			return err
		}
		genderResult = *result
		return nil
	})
	g.Go(func() error {
		result, err := s.api.FetchAge(name)
		if err != nil {
			return err
		}
		ageResult = *result
		return nil
	})
	g.Go(func() error {
		result, err := s.api.FetchNationality(name)
		if err != nil {
			return err
		}
		natResult = *result
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, nil, nil, err
	}

	return &genderResult, &ageResult, &natResult, nil
}
