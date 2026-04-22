package services

import "errors"

var (
	ErrNoPrediction       = errors.New("no prediction available")
	ErrNoGenderPrediction = errors.New("no gender prediction available")
	ErrNoAgePrediction    = errors.New("no age prediction available")
	ErrNoCountryPrediction = errors.New("no country prediction available")
)
