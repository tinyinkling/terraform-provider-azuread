package serviceprincipals

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/manicminer/hamilton/models"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/msgraph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/services/serviceprincipals/parse"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
)

func servicePrincipalPasswordResourceCreateMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics { //nolint
	client := meta.(*clients.Client).ServicePrincipals.MsClient
	objectId := d.Get("service_principal_id").(string)

	credential, err := msgraph.PasswordCredentialForResource(d)
	if err != nil {
		attr := ""
		if kerr, ok := err.(msgraph.CredentialError); ok {
			attr = kerr.Attr()
		}
		return tf.ErrorDiagPathF(err, attr, "Generating password credentials for service principal with object ID %q", objectId)
	}

	id := parse.NewCredentialID(objectId, "password", *credential.KeyId)

	tf.LockByName(servicePrincipalResourceName, id.ObjectId)
	defer tf.UnlockByName(servicePrincipalResourceName, id.ObjectId)

	app, status, err := client.Get(ctx, id.ObjectId)
	if err != nil {
		if status == http.StatusNotFound {
			return tf.ErrorDiagPathF(nil, "service_principal_id", "Service principal with object ID %q was not found", id.ObjectId)
		}
		return tf.ErrorDiagPathF(err, "service_principal_id", "Retrieving service principal with object ID %q", id.ObjectId)
	}

	newCredentials := make([]models.PasswordCredential, 0)
	if app.PasswordCredentials != nil {
		for _, cred := range *app.PasswordCredentials {
			if cred.KeyId != nil && *cred.KeyId == *credential.KeyId {
				return tf.ImportAsExistsDiag("azuread_service_principal_password", id.String())
			}
			newCredentials = append(newCredentials, cred)
		}
	}

	newCredentials = append(newCredentials, *credential)

	properties := models.ServicePrincipal{
		ID:                  &id.ObjectId,
		PasswordCredentials: &newCredentials,
	}
	if _, err := client.Update(ctx, properties); err != nil {
		return tf.ErrorDiagF(err, "Adding password for service principal with object ID %q", id.ObjectId)
	}

	d.SetId(id.String())

	return servicePrincipalPasswordResourceReadMsGraph(ctx, d, meta)
}

func servicePrincipalPasswordResourceReadMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics { //nolint
	client := meta.(*clients.Client).ServicePrincipals.MsClient

	id, err := parse.PasswordID(d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "id", "Parsing password credential with ID %q", d.Id())
	}

	app, status, err := client.Get(ctx, id.ObjectId)
	if err != nil {
		if status == http.StatusNotFound {
			return tf.ErrorDiagPathF(nil, "service_principal_id", "Service principal with object ID %q was not found", id.ObjectId)
		}
		return tf.ErrorDiagPathF(err, "service_principal_id", "Retrieving service principal with object ID %q", id.ObjectId)
	}

	var credential *models.PasswordCredential
	if app.PasswordCredentials != nil {
		for _, cred := range *app.PasswordCredentials {
			if cred.KeyId != nil && *cred.KeyId == id.KeyId {
				credential = &cred
				break
			}
		}
	}

	if credential == nil {
		log.Printf("[DEBUG] Password credential %q (ID %q) was not found - removing from state!", id.KeyId, id.ObjectId)
		d.SetId("")
		return nil
	}

	tf.Set(d, "description", credential.DisplayName)
	tf.Set(d, "display_name", credential.DisplayName)
	tf.Set(d, "key_id", id.KeyId)
	tf.Set(d, "service_principal_id", id.ObjectId)

	startDate := ""
	if v := credential.StartDateTime; v != nil {
		startDate = v.Format(time.RFC3339)
	}
	tf.Set(d, "start_date", startDate)

	endDate := ""
	if v := credential.EndDateTime; v != nil {
		endDate = v.Format(time.RFC3339)
	}
	tf.Set(d, "end_date", endDate)

	return nil
}

func servicePrincipalPasswordResourceDeleteMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics { //nolint
	client := meta.(*clients.Client).ServicePrincipals.MsClient

	id, err := parse.PasswordID(d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "id", "Parsing password credential with ID %q", d.Id())
	}

	tf.LockByName(servicePrincipalResourceName, id.ObjectId)
	defer tf.UnlockByName(servicePrincipalResourceName, id.ObjectId)

	app, status, err := client.Get(ctx, id.ObjectId)
	if err != nil {
		if status == http.StatusNotFound {
			log.Printf("[DEBUG] Service Principal with Object ID %q was not found - removing from state!", id.ObjectId)
			return nil
		}
		return tf.ErrorDiagPathF(err, "service_principal_id", "Retrieving service principal with object ID %q", id.ObjectId)
	}

	newCredentials := make([]models.PasswordCredential, 0)
	if app.PasswordCredentials != nil {
		for _, cred := range *app.PasswordCredentials {
			if cred.KeyId != nil && *cred.KeyId != id.KeyId {
				newCredentials = append(newCredentials, cred)
			}
		}
	}

	properties := models.ServicePrincipal{
		ID:                  &id.ObjectId,
		PasswordCredentials: &newCredentials,
	}
	if _, err := client.Update(ctx, properties); err != nil {
		return tf.ErrorDiagF(err, "Removing password credential %q from service principal with object ID %q", id.KeyId, id.ObjectId)
	}

	return nil
}
