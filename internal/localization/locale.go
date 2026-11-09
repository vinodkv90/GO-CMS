package localization

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Direction represents text layout direction (LTR / RTL).
type Direction string

const (
	DirectionLTR Direction = "ltr"
	DirectionRTL Direction = "rtl"
)

// Locale represents a language and cultural configuration.
type Locale struct {
	Code      string    `json:"code"` // e.g. "en", "es", "fr", "ar"
	Name      string    `json:"name"` // e.g. "English", "Spanish"
	IsDefault bool      `json:"is_default"`
	Direction Direction `json:"direction"`
	CreatedAt time.Time `json:"created_at"`
}

var localeCodeRegex = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

// Validate checks that a locale's code and properties are valid.
func (l *Locale) Validate() error {
	l.Code = strings.TrimSpace(l.Code)
	l.Name = strings.TrimSpace(l.Name)

	if !localeCodeRegex.MatchString(l.Code) {
		return fmt.Errorf("localization: invalid locale code %q (expected ISO format like 'en', 'es', 'zh-CN')", l.Code)
	}

	if l.Name == "" {
		return errors.New("localization: locale name cannot be empty")
	}

	if l.Direction != DirectionLTR && l.Direction != DirectionRTL {
		l.Direction = DirectionLTR
	}

	return nil
}

// Service manages active locales and provides content localization resolution.
type Service struct {
	mu            sync.RWMutex
	locales       map[string]Locale
	defaultLocale string
}

// NewService creates a new localization Service seeded with default "en" (English).
func NewService() *Service {
	s := &Service{
		locales:       make(map[string]Locale),
		defaultLocale: "en",
	}

	defaultLoc := Locale{
		Code:      "en",
		Name:      "English",
		IsDefault: true,
		Direction: DirectionLTR,
		CreatedAt: time.Now().UTC(),
	}
	s.locales["en"] = defaultLoc
	return s
}

// List returns all registered locales.
func (s *Service) List() []Locale {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]Locale, 0, len(s.locales))
	for _, l := range s.locales {
		res = append(res, l)
	}
	return res
}

// Get retrieves a locale by its code.
func (s *Service) Get(code string) (Locale, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	l, exists := s.locales[strings.ToLower(code)]
	if !exists {
		return Locale{}, fmt.Errorf("localization: locale %q not found", code)
	}
	return l, nil
}

// Add creates a new locale.
func (s *Service) Add(l Locale) (Locale, error) {
	if err := l.Validate(); err != nil {
		return Locale{}, err
	}

	code := strings.ToLower(l.Code)
	l.Code = code
	l.CreatedAt = time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.locales[code]; exists {
		return Locale{}, fmt.Errorf("localization: locale %q already exists", code)
	}

	if l.IsDefault {
		for k, existing := range s.locales {
			existing.IsDefault = false
			s.locales[k] = existing
		}
		s.defaultLocale = code
	}

	s.locales[code] = l
	return l, nil
}

// SetDefault sets the designated primary fallback locale.
func (s *Service) SetDefault(code string) error {
	code = strings.ToLower(code)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.locales[code]; !exists {
		return fmt.Errorf("localization: locale %q not found", code)
	}

	for k, l := range s.locales {
		l.IsDefault = (k == code)
		s.locales[k] = l
	}
	s.defaultLocale = code
	return nil
}

// Delete removes a non-default locale.
func (s *Service) Delete(code string) error {
	code = strings.ToLower(code)

	s.mu.Lock()
	defer s.mu.Unlock()

	l, exists := s.locales[code]
	if !exists {
		return fmt.Errorf("localization: locale %q not found", code)
	}

	if l.IsDefault {
		return errors.New("localization: cannot delete the default locale")
	}

	delete(s.locales, code)
	return nil
}

// DefaultLocale returns the active default fallback locale code.
func (s *Service) DefaultLocale() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultLocale
}

// ResolveLocalizedData resolves a content entry dictionary for a target locale,
// falling back to the default locale when a field translation is not provided.
func (s *Service) ResolveLocalizedData(data map[string]any, targetLocale string) map[string]any {
	if data == nil {
		return make(map[string]any)
	}

	target := strings.ToLower(targetLocale)
	fallback := s.DefaultLocale()

	result := make(map[string]any, len(data))
	for k, v := range data {
		// If value is a map with language keys: {"en": "...", "es": "..."}
		if translations, ok := v.(map[string]any); ok {
			if val, hasTarget := translations[target]; hasTarget && val != nil && val != "" {
				result[k] = val
			} else if val, hasFallback := translations[fallback]; hasFallback && val != nil && val != "" {
				result[k] = val
			} else {
				result[k] = v
			}
		} else {
			// Plain non-localized field
			result[k] = v
		}
	}

	return result
}
