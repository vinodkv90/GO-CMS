package localization

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Direction represents text layout direction (LTR / RTL).
type Direction string

const (
	DirectionLTR Direction = "ltr"
	DirectionRTL Direction = "rtl"
)

// Locale represents a language and cultural configuration.
type Locale struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`        // e.g. "en", "es", "fr", "ar", "zh-CN"
	Name         string    `json:"name"`        // e.g. "English", "Spanish"
	NativeName   string    `json:"native_name"` // e.g. "English", "Español"
	IsDefault    bool      `json:"is_default"`
	IsEnabled    bool      `json:"is_enabled"`
	FallbackCode string    `json:"fallback_code,omitempty"`
	Direction    Direction `json:"direction"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LocaleUpdate captures mutable properties of an existing locale.
type LocaleUpdate struct {
	Name         *string    `json:"name"`
	NativeName   *string    `json:"native_name"`
	IsDefault    *bool      `json:"is_default"`
	IsEnabled    *bool      `json:"is_enabled"`
	FallbackCode *string    `json:"fallback_code"`
	Direction    *Direction `json:"direction"`
}

// InUseChecker reports whether a locale code is referenced by content entries or settings.
type InUseChecker func(ctx context.Context, code string) (bool, error)

var localeCodeRegex = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

// Validate checks that a locale's code and properties are valid.
func (l *Locale) Validate() error {
	l.Code = strings.TrimSpace(l.Code)
	l.Name = strings.TrimSpace(l.Name)
	l.NativeName = strings.TrimSpace(l.NativeName)
	l.FallbackCode = strings.TrimSpace(l.FallbackCode)

	if !localeCodeRegex.MatchString(l.Code) {
		return fmt.Errorf("localization: invalid locale code %q (expected ISO format like 'en', 'es', 'zh-CN')", l.Code)
	}

	if l.Name == "" {
		return errors.New("localization: locale name cannot be empty")
	}
	if l.NativeName == "" {
		l.NativeName = l.Name
	}

	if l.Direction != DirectionLTR && l.Direction != DirectionRTL {
		l.Direction = DirectionLTR
	}

	return nil
}

// Service manages active locales and provides content localization resolution.
type Service struct {
	mu           sync.RWMutex
	repo         Repository
	inUseChecker InUseChecker
}

// NewService creates a new localization Service backed by an in-memory repository seeded with "en".
func NewService() *Service {
	return NewServiceWithRepo(NewMemoryRepository())
}

// NewServiceWithRepo creates a new localization Service backed by a persistent Repository.
func NewServiceWithRepo(repo Repository) *Service {
	if repo == nil {
		repo = NewMemoryRepository()
	}
	s := &Service{repo: repo}
	_ = s.EnsureSeeded(context.Background())
	return s
}

// SetInUseChecker registers a callback checked before deleting a locale.
func (s *Service) SetInUseChecker(fn InUseChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inUseChecker = fn
}

// EnsureSeeded ensures at least one default "en" locale exists in storage.
func (s *Service) EnsureSeeded(ctx context.Context) error {
	list, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	if len(list) > 0 {
		return nil
	}
	now := time.Now().UTC()
	return s.repo.Create(ctx, Locale{
		ID:         uuid.NewString(),
		Code:       "en",
		Name:       "English",
		NativeName: "English",
		IsDefault:  true,
		IsEnabled:  true,
		Direction:  DirectionLTR,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
}

// List returns all registered locales.
func (s *Service) List() []Locale {
	list, err := s.repo.List(context.Background())
	if err != nil {
		return []Locale{}
	}
	return list
}

// Get retrieves a locale by its code.
func (s *Service) Get(code string) (Locale, error) {
	l, err := s.repo.GetByCode(context.Background(), code)
	if err != nil {
		return Locale{}, fmt.Errorf("localization: locale %q not found", code)
	}
	return l, nil
}

// Add creates a new locale.
func (s *Service) Add(l Locale) (Locale, error) {
	if err := l.Validate(); err != nil {
		return Locale{}, err
	}

	now := time.Now().UTC()
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	if !l.IsEnabled {
		// Default new locales to enabled unless updated explicitly via Update
		l.IsEnabled = true
	}
	l.CreatedAt = now
	l.UpdatedAt = now

	if l.FallbackCode != "" && !strings.EqualFold(l.FallbackCode, l.Code) {
		if _, err := s.repo.GetByCode(context.Background(), l.FallbackCode); err != nil {
			return Locale{}, fmt.Errorf("localization: fallback locale %q does not exist", l.FallbackCode)
		}
	} else if strings.EqualFold(l.FallbackCode, l.Code) {
		l.FallbackCode = ""
	}

	if err := s.repo.Create(context.Background(), l); err != nil {
		return Locale{}, err
	}
	return l, nil
}

// Update modifies an existing locale configuration.
func (s *Service) Update(code string, upd LocaleUpdate) (Locale, error) {
	existing, err := s.repo.GetByCode(context.Background(), code)
	if err != nil {
		return Locale{}, fmt.Errorf("localization: locale %q not found", code)
	}

	if upd.Name != nil {
		existing.Name = strings.TrimSpace(*upd.Name)
		if existing.Name == "" {
			return Locale{}, errors.New("localization: locale name cannot be empty")
		}
	}
	if upd.NativeName != nil {
		existing.NativeName = strings.TrimSpace(*upd.NativeName)
		if existing.NativeName == "" {
			existing.NativeName = existing.Name
		}
	}
	if upd.Direction != nil {
		if *upd.Direction != DirectionLTR && *upd.Direction != DirectionRTL {
			return Locale{}, fmt.Errorf("localization: invalid direction %q", *upd.Direction)
		}
		existing.Direction = *upd.Direction
	}
	if upd.FallbackCode != nil {
		fb := strings.TrimSpace(*upd.FallbackCode)
		if fb != "" {
			if strings.EqualFold(fb, existing.Code) {
				return Locale{}, errors.New("localization: fallback locale cannot reference itself")
			}
			if _, err := s.repo.GetByCode(context.Background(), fb); err != nil {
				return Locale{}, fmt.Errorf("localization: fallback locale %q does not exist", fb)
			}
		}
		existing.FallbackCode = fb
	}
	if upd.IsDefault != nil && *upd.IsDefault {
		existing.IsDefault = true
		existing.IsEnabled = true
	}
	if upd.IsEnabled != nil {
		if existing.IsDefault && !*upd.IsEnabled {
			return Locale{}, errors.New("localization: cannot disable the default locale")
		}
		existing.IsEnabled = *upd.IsEnabled
	}
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(context.Background(), existing); err != nil {
		return Locale{}, err
	}
	return existing, nil
}

// SetDefault sets the designated primary fallback locale.
func (s *Service) SetDefault(code string) error {
	if err := s.repo.SetDefault(context.Background(), code); err != nil {
		return fmt.Errorf("localization: %w", err)
	}
	return nil
}

// Delete removes a non-default, unreferenced locale.
func (s *Service) Delete(code string) error {
	ctx := context.Background()
	l, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("localization: locale %q not found", code)
	}
	if l.IsDefault {
		return errors.New("localization: cannot delete the default locale")
	}

	s.mu.RLock()
	checker := s.inUseChecker
	s.mu.RUnlock()
	if checker != nil {
		inUse, err := checker(ctx, l.Code)
		if err != nil {
			return fmt.Errorf("localization: failed to verify locale usage: %w", err)
		}
		if inUse {
			return fmt.Errorf("%w: locale %q is referenced by existing content entries or settings", ErrLocaleInUse, l.Code)
		}
	}

	return s.repo.Delete(ctx, l.Code)
}

// DefaultLocale returns the active default fallback locale code.
func (s *Service) DefaultLocale() string {
	list, err := s.repo.List(context.Background())
	if err == nil {
		for _, l := range list {
			if l.IsDefault {
				return l.Code
			}
		}
	}
	return "en"
}

// ResolveLocalizedData resolves a content entry dictionary for a target locale,
// falling back through the locale's configured FallbackCode and finally DefaultLocale.
func (s *Service) ResolveLocalizedData(data map[string]any, targetLocale string) map[string]any {
	if data == nil {
		return make(map[string]any)
	}

	target := strings.ToLower(targetLocale)
	fallback := strings.ToLower(s.DefaultLocale())
	var configuredFallback string
	if targetLoc, err := s.Get(targetLocale); err == nil && targetLoc.FallbackCode != "" {
		configuredFallback = strings.ToLower(targetLoc.FallbackCode)
	}

	result := make(map[string]any, len(data))
	for k, v := range data {
		if translations, ok := v.(map[string]any); ok {
			if val, hasTarget := translations[target]; hasTarget && val != nil && val != "" {
				result[k] = val
			} else if configuredFallback != "" && hasNonEmptyKey(translations, configuredFallback) {
				result[k] = translations[configuredFallback]
			} else if val, hasFallback := translations[fallback]; hasFallback && val != nil && val != "" {
				result[k] = val
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}

	return result
}

func hasNonEmptyKey(m map[string]any, k string) bool {
	val, ok := m[k]
	return ok && val != nil && val != ""
}
