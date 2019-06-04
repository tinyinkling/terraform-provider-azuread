package azuread

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/terraform-providers/terraform-provider-azuread/azuread/helpers/tf"

	"github.com/terraform-providers/terraform-provider-azuread/azuread/helpers/validate"

	"github.com/hashicorp/go-azure-helpers/response"
	"github.com/terraform-providers/terraform-provider-azuread/azuread/helpers/ar"
	"github.com/terraform-providers/terraform-provider-azuread/azuread/helpers/p"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/hashicorp/terraform/helper/schema"
)

const servicePrincipalResourceName = "azuread_service_principal"

func resourceServicePrincipal() *schema.Resource {
	return &schema.Resource{
		Create: resourceServicePrincipalCreate,
		Read:   resourceServicePrincipalRead,
		Delete: resourceServicePrincipalDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"application_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.UUID,
			},

			"tags": {
				Type:     schema.TypeSet,
				Optional: true,
				Set:      schema.HashString,
				ForceNew: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"display_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceServicePrincipalCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).servicePrincipalsClient
	ctx := meta.(*ArmClient).StopContext

	applicationId := d.Get("application_id").(string)

	properties := graphrbac.ServicePrincipalCreateParameters{
		AppID: p.String(applicationId),
		// there's no way of retrieving this, and there's no way of changing it
		// given there's no way to change it - we'll just default this to true
		AccountEnabled: p.Bool(true),
	}
	if v, ok := d.GetOk("tags"); ok {
		properties.Tags = tf.ExpandStringSlicePtr(v.(*schema.Set).List())
	}

	sp, err := client.Create(ctx, properties)
	if err != nil {
		return fmt.Errorf("Error creating Service Principal for application  %q: %+v", applicationId, err)
	}
	if sp.ObjectID == nil {
		return fmt.Errorf("Service Principal	objectID is nil")
	}
	d.SetId(*sp.ObjectID)

	i, err := (&resource.StateChangeConf{
		Pending:                   []string{"404"},
		Target:                    []string{"Found"},
		Timeout:                   azureAdReplicationTimeout,
		MinTimeout:                1 * time.Second,
		ContinuousTargetOccurence: azureAdReplicationTargetOccurence,
		Refresh: func() (interface{}, string, error) {
			resp, err2 := client.Get(ctx, *sp.ObjectID)
			if err2 != nil {
				if ar.ResponseWasNotFound(resp.Response) {
					return resp, "404", nil
				}
				return resp, "Error", fmt.Errorf("Error retrieving Service Principal ID %q: %+v", *sp.ObjectID, err2)
			}

			return resp, "Found", nil
		},
	}).WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for application: %+v", err)
	}
	sp = i.(graphrbac.ServicePrincipal)

	return resourceServicePrincipalRead(d, meta)
}

func resourceServicePrincipalRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).servicePrincipalsClient
	ctx := meta.(*ArmClient).StopContext

	objectId := d.Id()

	app, err := client.Get(ctx, objectId)
	if err != nil {
		if ar.ResponseWasNotFound(app.Response) {
			log.Printf("[DEBUG] Service Principal with Object ID %q was not found - removing from state!", objectId)
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error retrieving Service Principal ID %q: %+v", objectId, err)
	}

	d.Set("application_id", app.AppID)
	d.Set("display_name", app.DisplayName)

	// tags doesn't exist as a property, so extract it
	if iTags, ok := app.AdditionalProperties["tags"]; ok {
		if tags, ok := iTags.([]interface{}); ok {
			if err := d.Set("tags", tf.ExpandStringSlicePtr(tags)); err != nil {
				return fmt.Errorf("Error setting `tags`: %+v", err)
			}
		}
	}

	return nil
}

func resourceServicePrincipalDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).servicePrincipalsClient
	ctx := meta.(*ArmClient).StopContext

	applicationId := d.Id()
	app, err := client.Delete(ctx, applicationId)
	if err != nil {
		if !response.WasNotFound(app.Response) {
			return fmt.Errorf("Error deleting Service Principal ID %q: %+v", applicationId, err)
		}
	}

	return nil
}
