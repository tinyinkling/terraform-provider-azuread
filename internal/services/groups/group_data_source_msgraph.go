package groups

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/manicminer/hamilton/models"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
)

func groupDataSourceReadMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	var group models.Group
	var displayName string

	if v, ok := d.GetOk("display_name"); ok {
		displayName = v.(string)
	} else if v, ok := d.GetOk("name"); ok {
		displayName = v.(string)
	}

	if displayName != "" {
		filter := fmt.Sprintf("displayName eq '%s'", displayName)
		groups, _, err := client.List(ctx, filter)
		if err != nil {
			return tf.ErrorDiagF(err, "Finding group with display name: %q", displayName)
		}

		count := len(*groups)
		if count > 1 {
			return tf.ErrorDiagPathF(nil, "name", "More than one group found with display name: %q", displayName)
		} else if count == 0 {
			return tf.ErrorDiagPathF(err, "name", "No group found with display name: %q", displayName)
		}

		group = (*groups)[0]
	} else if objectId, ok := d.Get("object_id").(string); ok && objectId != "" {
		g, status, err := client.Get(ctx, objectId)
		if err != nil {
			if status == http.StatusNotFound {
				return tf.ErrorDiagPathF(nil, "object_id", "No group found with object ID: %q", objectId)
			}
			return tf.ErrorDiagF(err, "Retrieving group with object ID: %q", objectId)
		}
		if g == nil {
			return tf.ErrorDiagPathF(nil, "object_id", "Group not found with object ID: %q", objectId)
		}
		group = *g
	}

	if group.ID == nil {
		return tf.ErrorDiagF(errors.New("API returned group with nil object ID"), "Bad API Response")
	}

	d.SetId(*group.ID)

	tf.Set(d, "description", group.Description)
	tf.Set(d, "display_name", group.DisplayName)
	tf.Set(d, "name", group.DisplayName) // TODO: v2.0 remove this
	tf.Set(d, "object_id", group.ID)

	members, _, err := client.ListMembers(ctx, d.Id())
	if err != nil {
		return tf.ErrorDiagF(err, "Could not retrieve group members for group with object ID: %q", d.Id())
	}
	tf.Set(d, "members", members)

	owners, _, err := client.ListOwners(ctx, d.Id())
	if err != nil {
		return tf.ErrorDiagF(err, "Could not retrieve group owners for group with object ID: %q", d.Id())
	}
	tf.Set(d, "owners", owners)

	return nil
}
