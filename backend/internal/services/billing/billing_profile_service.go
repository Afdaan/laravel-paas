package billing

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

var (
	ErrInvalidBillingEmail    = errors.New("invalid billing email format")
	ErrInvalidBillingProfile  = errors.New("invalid billing profile")
	ErrBillingProfileRequired = errors.New("complete billing profile required")
)

type BillingProfileService struct {
	db *gorm.DB
}

func NewBillingProfileService(db *gorm.DB) *BillingProfileService {
	return &BillingProfileService{db: db}
}

func (s *BillingProfileService) GetProfile(ctx context.Context, userID uint) (models.BillingProfile, error) {
	var profile models.BillingProfile
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var user models.User
			if errUser := s.db.WithContext(ctx).First(&user, userID).Error; errUser == nil {
				return models.BillingProfile{
					UserID:      userID,
					CompanyName: user.Name,
					Email:       user.Email,
					Country:     "ID",
				}, nil
			}
			return models.BillingProfile{UserID: userID, Country: "ID"}, nil
		}
		return models.BillingProfile{}, fmt.Errorf("get billing profile: %w", err)
	}
	return profile, nil
}

type UpdateBillingProfileInput struct {
	CompanyName   string `json:"company_name"`
	TaxID         string `json:"tax_id"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	AddressLine1  string `json:"address_line1"`
	AddressLine2  string `json:"address_line2"`
	City          string `json:"city"`
	StateProvince string `json:"state_province"`
	PostalCode    string `json:"postal_code"`
	Country       string `json:"country"`
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
var billingProfileDigits = regexp.MustCompile(`\D`)
var billingPostalCodeRegex = regexp.MustCompile(`^[0-9]{5}$`)

func normalizeBillingProfileInput(input UpdateBillingProfileInput) UpdateBillingProfileInput {
	input.CompanyName = strings.TrimSpace(input.CompanyName)
	input.TaxID = strings.TrimSpace(input.TaxID)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.AddressLine1 = strings.TrimSpace(input.AddressLine1)
	input.AddressLine2 = strings.TrimSpace(input.AddressLine2)
	input.City = strings.TrimSpace(input.City)
	input.StateProvince = strings.TrimSpace(input.StateProvince)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.Country = strings.ToUpper(strings.TrimSpace(input.Country))
	if input.Country == "IDN" {
		input.Country = "ID"
	}
	return input
}

func validBillingProfileInput(input UpdateBillingProfileInput) bool {
	if input.Country != "ID" || utf8.RuneCountInString(input.CompanyName) < 2 || utf8.RuneCountInString(input.CompanyName) > 255 ||
		input.Email == "" || utf8.RuneCountInString(input.Email) > 255 || !emailRegex.MatchString(input.Email) ||
		utf8.RuneCountInString(input.Phone) > 50 || utf8.RuneCountInString(input.AddressLine1) < 5 || utf8.RuneCountInString(input.AddressLine1) > 255 ||
		utf8.RuneCountInString(input.AddressLine2) > 255 || utf8.RuneCountInString(input.City) < 2 || utf8.RuneCountInString(input.City) > 100 ||
		utf8.RuneCountInString(input.StateProvince) > 100 || utf8.RuneCountInString(input.PostalCode) > 20 || utf8.RuneCountInString(input.TaxID) > 100 {
		return false
	}
	phoneDigits := billingProfileDigits.ReplaceAllString(input.Phone, "")
	validPhone := (strings.HasPrefix(phoneDigits, "08") && len(phoneDigits) >= 10 && len(phoneDigits) <= 13) ||
		(strings.HasPrefix(phoneDigits, "628") && len(phoneDigits) >= 11 && len(phoneDigits) <= 14) ||
		(strings.HasPrefix(phoneDigits, "8") && len(phoneDigits) >= 9 && len(phoneDigits) <= 12)
	if !validPhone || !billingPostalCodeRegex.MatchString(input.PostalCode) {
		return false
	}
	if input.TaxID != "" && len(billingProfileDigits.ReplaceAllString(input.TaxID, "")) < 15 {
		return false
	}
	return true
}

func (s *BillingProfileService) UpsertProfile(ctx context.Context, userID uint, input UpdateBillingProfileInput) (models.BillingProfile, error) {
	if s == nil || s.db == nil || ctx == nil || userID == 0 {
		return models.BillingProfile{}, ErrInvalidBillingProfile
	}
	input = normalizeBillingProfileInput(input)
	if !validBillingProfileInput(input) {
		if input.Email != "" && !emailRegex.MatchString(input.Email) {
			return models.BillingProfile{}, ErrInvalidBillingEmail
		}
		return models.BillingProfile{}, ErrInvalidBillingProfile
	}

	var profile models.BillingProfile
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		errFetch := tx.Where("user_id = ?", userID).First(&profile).Error
		if errFetch != nil && !errors.Is(errFetch, gorm.ErrRecordNotFound) {
			return errFetch
		}

		profile.UserID = userID
		profile.CompanyName = input.CompanyName
		profile.TaxID = input.TaxID
		profile.Email = input.Email
		profile.Phone = input.Phone
		profile.AddressLine1 = input.AddressLine1
		profile.AddressLine2 = input.AddressLine2
		profile.City = input.City
		profile.StateProvince = input.StateProvince
		profile.PostalCode = input.PostalCode
		profile.Country = input.Country

		if errFetch == nil {
			return tx.Save(&profile).Error
		}
		return tx.Create(&profile).Error
	})

	if err != nil {
		return models.BillingProfile{}, fmt.Errorf("upsert billing profile: %w", err)
	}

	return profile, nil
}

func (s *BillingProfileService) RequireComplete(ctx context.Context, userID uint) error {
	if s == nil || s.db == nil || ctx == nil || userID == 0 {
		return ErrBillingProfileRequired
	}
	var profile models.BillingProfile
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBillingProfileRequired
		}
		return fmt.Errorf("load billing profile readiness: %w", err)
	}
	input := normalizeBillingProfileInput(UpdateBillingProfileInput{
		CompanyName: profile.CompanyName, TaxID: profile.TaxID, Email: profile.Email, Phone: profile.Phone,
		AddressLine1: profile.AddressLine1, AddressLine2: profile.AddressLine2, City: profile.City,
		StateProvince: profile.StateProvince, PostalCode: profile.PostalCode, Country: profile.Country,
	})
	if !validBillingProfileInput(input) {
		return ErrBillingProfileRequired
	}
	return nil
}

func CountryCodeToISO3(country2 string) string {
	switch country2 {
	case "ID", "IDN":
		return "IDN"
	case "US", "USA":
		return "USA"
	case "SG", "SGP":
		return "SGP"
	case "MY", "MYS":
		return "MYS"
	default:
		if len(country2) == 3 {
			return country2
		}
		return "IDN"
	}
}
