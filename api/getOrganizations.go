package api

import (
	"github.com/gin-gonic/gin"
	"strings"
)

type OrganizationPlan struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type Organization struct {
	Slug                       string           `json:"slug"`
	Name                       string           `json:"name"`
	StripeCustomerId           interface{}      `json:"stripe_customer_id"`
	SubscriptionId             interface{}      `json:"subscription_id"`
	BillingEmail               interface{}      `json:"billing_email"`
	BillingPartner             interface{}      `json:"billing_partner"`
	IsOwner                    bool             `json:"is_owner"`
	OptInTags                  []string         `json:"opt_in_tags"`
	Id                         int32            `json:"id"`
	RestrictionData            interface{}      `json:"restriction_data"`
	RestrictionStatus          interface{}      `json:"restriction_status"`
	Plan                       OrganizationPlan `json:"plan"`
	UsageBillingEnabled        bool             `json:"usage_billing_enabled"`
	OrganizationMissingAddress bool             `json:"organization_missing_address"`
	OrganizationMissingTaxId   bool             `json:"organization_missing_tax_id"`
	OrganizationRequiresMfa    bool             `json:"organization_requires_mfa"`
}

func (a *Api) getOrganizations(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	orgs, err := a.queries.GetOrganizationsForAccountId(c, account.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	supaOrgs := []Organization{}
	for _, org := range orgs {
		supaOrgs = append(supaOrgs, Organization{
			Slug:             org.Slug,
			Name:             org.Name,
			StripeCustomerId: nil,
			SubscriptionId:   nil,
			BillingEmail:     "billing@supadash.io",
			BillingPartner:   nil,
			IsOwner:          strings.ToLower(org.MemberRole) == "owner",
			OptInTags:        []string{},
			Id:               org.ID,
			RestrictionData:  nil,
			RestrictionStatus: nil,
			Plan: OrganizationPlan{
				Id:   "enterprise",
				Name: "Enterprise",
			},
			UsageBillingEnabled:        false,
			OrganizationMissingAddress: false,
			OrganizationMissingTaxId:   false,
			OrganizationRequiresMfa:    false,
		})
	}

	c.JSON(200, supaOrgs)
}
