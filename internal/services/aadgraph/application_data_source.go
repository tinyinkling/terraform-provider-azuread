package aadgraph

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/services/aadgraph/graph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
	"github.com/terraform-providers/terraform-provider-azuread/internal/utils"
	"github.com/terraform-providers/terraform-provider-azuread/internal/validate"
)

func applicationData() *schema.Resource {
	return &schema.Resource{
		Read: applicationDataRead,

		Schema: map[string]*schema.Schema{
			"object_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"application_id", "name", "object_id"},
				ValidateFunc: validate.UUID,
			},

			"application_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"application_id", "name", "object_id"},
				ValidateFunc: validate.UUID,
			},

			"name": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"application_id", "name", "object_id"},
				ValidateFunc: validate.NoEmptyStrings,
			},

			"homepage": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"identifier_uris": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"reply_urls": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"logout_url": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"available_to_other_tenants": {
				Type:     schema.TypeBool,
				Computed: true,
			},

			"oauth2_allow_implicit_flow": {
				Type:     schema.TypeBool,
				Computed: true,
			},

			"group_membership_claims": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"type": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"app_roles": graph.SchemaAppRolesComputed(),

			"optional_claims": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_token": graph.SchemaOptionalClaims(),
						"id_token":     graph.SchemaOptionalClaims(),
						// TODO: enable when https://github.com/Azure/azure-sdk-for-go/issues/9714 resolved
						//"saml_token": graph.SchemaOptionalClaims(),
					},
				},
			},

			"required_resource_access": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"resource_app_id": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"resource_access": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": {
										Type:     schema.TypeString,
										Computed: true,
									},

									"type": {
										Type:     schema.TypeString,
										Computed: true,
									},
								},
							},
						},
					},
				},
			},

			"owners": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"oauth2_permissions": graph.SchemaOauth2PermissionsComputed(),
		},
	}
}

func applicationDataRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.AadClient).AadGraph.ApplicationsClient
	ctx := meta.(*clients.AadClient).StopContext

	var app graphrbac.Application

	if oId, ok := d.Get("object_id").(string); ok && oId != "" {
		resp, err := client.Get(ctx, oId)
		if err != nil {
			if utils.ResponseWasNotFound(resp.Response) {
				return fmt.Errorf("Application with ID %q was not found", oId)
			}

			return fmt.Errorf("retrieving Application with ID %q: %+v", oId, err)
		}

		app = resp
	} else {
		var fieldName, fieldValue string
		if applicationId, ok := d.Get("application_id").(string); ok && applicationId != "" {
			fieldName = "appId"
			fieldValue = applicationId
		} else if name, ok := d.Get("name").(string); ok && name != "" {
			fieldName = "displayName"
			fieldValue = name
		} else {
			return fmt.Errorf("one of `object_id` or `name` must be supplied")
		}

		filter := fmt.Sprintf("%s eq '%s'", fieldName, fieldValue)

		resp, err := client.ListComplete(ctx, filter)
		if err != nil {
			return fmt.Errorf("listing Applications for filter %q: %+v", filter, err)
		}

		values := resp.Response().Value
		if values == nil {
			return fmt.Errorf("bad API response: nil values for Applications matching %q", filter)
		}
		if len(*values) == 0 {
			return fmt.Errorf("found no Applications matching %q", filter)
		}
		if len(*values) > 1 {
			return fmt.Errorf("found multiple Applications matching %q", filter)
		}

		app = (*values)[0]
		switch fieldName {
		case "appId":
			if app.AppID == nil {
				return fmt.Errorf("bad API response: nil AppID for Applications matching %q", filter)
			}
			if *app.AppID != fieldValue {
				return fmt.Errorf("AppID for Applications matching %q does not match(%q!=%q)", filter, *app.AppID, fieldValue)
			}
		case "displayName":
			if app.DisplayName == nil {
				return fmt.Errorf("nil DisplayName for Applications matching %q", filter)
			}
			if *app.DisplayName != fieldValue {
				return fmt.Errorf("DisplayName for Applications matching %q does not match(%q!=%q)", filter, *app.DisplayName, fieldValue)
			}
		}
	}

	if app.ObjectID == nil {
		return fmt.Errorf("Application ObjectId is nil")
	}
	d.SetId(*app.ObjectID)

	d.Set("object_id", app.ObjectID)
	d.Set("name", app.DisplayName)
	d.Set("application_id", app.AppID)
	d.Set("homepage", app.Homepage)
	d.Set("logout_url", app.LogoutURL)
	d.Set("available_to_other_tenants", app.AvailableToOtherTenants)
	d.Set("oauth2_allow_implicit_flow", app.Oauth2AllowImplicitFlow)

	if err := d.Set("identifier_uris", tf.FlattenStringSlicePtr(app.IdentifierUris)); err != nil {
		return fmt.Errorf("setting `identifier_uris`: %+v", err)
	}

	if err := d.Set("reply_urls", tf.FlattenStringSlicePtr(app.ReplyUrls)); err != nil {
		return fmt.Errorf("setting `reply_urls`: %+v", err)
	}

	if err := d.Set("required_resource_access", flattenApplicationRequiredResourceAccess(app.RequiredResourceAccess)); err != nil {
		return fmt.Errorf("setting `required_resource_access`: %+v", err)
	}

	if err := d.Set("optional_claims", flattenApplicationOptionalClaims(app.OptionalClaims)); err != nil {
		return fmt.Errorf("setting `optional_claims`: %+v", err)
	}

	if v := app.PublicClient; v != nil && *v {
		d.Set("type", "native")
	} else {
		d.Set("type", "webapp/api")
	}

	if err := d.Set("app_roles", graph.FlattenAppRoles(app.AppRoles)); err != nil {
		return fmt.Errorf("setting `app_roles`: %+v", err)
	}

	if err := d.Set("group_membership_claims", app.GroupMembershipClaims); err != nil {
		return fmt.Errorf("setting `group_membership_claims`: %+v", err)
	}

	if err := d.Set("oauth2_permissions", graph.FlattenOauth2Permissions(app.Oauth2Permissions)); err != nil {
		return fmt.Errorf("setting `oauth2_permissions`: %+v", err)
	}

	owners, err := graph.ApplicationAllOwners(ctx, client, d.Id())
	if err != nil {
		return fmt.Errorf("getting owners for Application %q: %+v", *app.ObjectID, err)
	}
	if err := d.Set("owners", owners); err != nil {
		return fmt.Errorf("setting `owners`: %+v", err)
	}

	return nil
}