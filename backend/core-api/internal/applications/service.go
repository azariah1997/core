package applications

import "context"

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

// Service is the application-layer entry point: it enforces domain
// invariants and delegates persistence to a Repository. HTTP handlers and
// any future SDK/gRPC surface should depend on this, not on Repository
// directly.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Application, error) {
	if err := in.Validate(); err != nil {
		return Application{}, err
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Get(ctx context.Context, id string) (Application, error) {
	if !ValidID(id) {
		return Application{}, &ValidationError{"id must be a valid UUID"}
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, params ListParams) (ListResult, error) {
	if params.Limit <= 0 || params.Limit > maxListLimit {
		params.Limit = defaultListLimit
	}
	return s.repo.List(ctx, params)
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (Application, error) {
	if !ValidID(id) {
		return Application{}, &ValidationError{"id must be a valid UUID"}
	}
	if err := in.Validate(); err != nil {
		return Application{}, err
	}
	return s.repo.Update(ctx, id, in)
}
