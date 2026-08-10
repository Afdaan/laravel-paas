package billing

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

var (
	ErrInvalidBillingEmail = errors.New("invalid billing email format")
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

func (s *BillingProfileService) UpsertProfile(ctx context.Context, userID uint, input UpdateBillingProfileInput) (models.BillingProfile, error) {
	if input.Email != "" && !emailRegex.MatchString(input.Email) {
		return models.BillingProfile{}, ErrInvalidBillingEmail
	}

	country := input.Country
	if country == "" {
		country = "ID"
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
		profile.Country = country

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
