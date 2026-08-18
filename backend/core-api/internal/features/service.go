package features

import "context"

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service, no adapter needed. Feature/rule management is
// platform.admin only: flags are operational configuration, not
// per-user data.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

type Service struct {
	repo  Repository
	admin AdminChecker
}

func NewService(repo Repository, admin AdminChecker) *Service {
	return &Service{repo: repo, admin: admin}
}

func (s *Service) CreateFeature(ctx context.Context, callerID string, in CreateFeatureInput) (Feature, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return Feature{}, err
	}
	if err := in.Validate(); err != nil {
		return Feature{}, err
	}
	return s.repo.CreateFeature(ctx, in)
}

// GetFeature and ListFeatures/ListRules are open to any authenticated
// caller - flags aren't sensitive per-user data, and Evaluate needs the
// same read anyway.
func (s *Service) GetFeature(ctx context.Context, appID, key string) (Feature, error) {
	return s.repo.GetFeature(ctx, appID, key)
}

func (s *Service) ListFeatures(ctx context.Context, appID string) ([]Feature, error) {
	return s.repo.ListFeatures(ctx, appID)
}

func (s *Service) UpdateFeature(ctx context.Context, callerID, appID, key string, in UpdateFeatureInput) (Feature, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return Feature{}, err
	}
	if err := in.Validate(); err != nil {
		return Feature{}, err
	}
	return s.repo.UpdateFeature(ctx, appID, key, in)
}

func (s *Service) CreateRule(ctx context.Context, callerID, appID, key string, in CreateRuleInput) (FeatureRule, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return FeatureRule{}, err
	}
	if err := in.Validate(); err != nil {
		return FeatureRule{}, err
	}
	feature, err := s.repo.GetFeature(ctx, appID, key)
	if err != nil {
		return FeatureRule{}, err
	}
	return s.repo.CreateRule(ctx, feature.ID, in)
}

func (s *Service) ListRules(ctx context.Context, appID, key string) ([]FeatureRule, error) {
	feature, err := s.repo.GetFeature(ctx, appID, key)
	if err != nil {
		return nil, err
	}
	return s.repo.ListRules(ctx, feature.ID)
}

func (s *Service) UpdateRule(ctx context.Context, callerID, appID, key, ruleID string, in UpdateRuleInput) (FeatureRule, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return FeatureRule{}, err
	}
	if err := in.Validate(); err != nil {
		return FeatureRule{}, err
	}
	if err := s.requireRuleBelongsToFeature(ctx, appID, key, ruleID); err != nil {
		return FeatureRule{}, err
	}
	return s.repo.UpdateRule(ctx, ruleID, in)
}

func (s *Service) DeleteRule(ctx context.Context, callerID, appID, key, ruleID string) error {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return err
	}
	if err := s.requireRuleBelongsToFeature(ctx, appID, key, ruleID); err != nil {
		return err
	}
	return s.repo.DeleteRule(ctx, ruleID)
}

// requireRuleBelongsToFeature stops a caller who can guess/enumerate a
// rule ID from operating on it through an unrelated feature's URL path -
// the rule must actually belong to the (appID, key) named in the request.
func (s *Service) requireRuleBelongsToFeature(ctx context.Context, appID, key, ruleID string) error {
	feature, err := s.repo.GetFeature(ctx, appID, key)
	if err != nil {
		return err
	}
	rule, err := s.repo.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}
	if rule.FeatureID != feature.ID {
		return ErrRuleNotFound
	}
	return nil
}

// Evaluate is "applications ask isEnabled(feature, context)" - the
// roadmap's own framing, and the whole reason this package exists.
func (s *Service) Evaluate(ctx context.Context, appID, key string, evalCtx EvaluationContext) (FeatureEvaluation, error) {
	feature, err := s.repo.GetFeature(ctx, appID, key)
	if err != nil {
		return FeatureEvaluation{}, err
	}
	rules, err := s.repo.ListRules(ctx, feature.ID)
	if err != nil {
		return FeatureEvaluation{}, err
	}
	return Evaluate(feature, rules, evalCtx), nil
}

func (s *Service) requireAdmin(ctx context.Context, callerID string) error {
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
