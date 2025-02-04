package mongodbatlas

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var integrationTypes = []string{
	"PAGER_DUTY",
	"DATADOG",
	"OPS_GENIE",
	"VICTOR_OPS",
	"WEBHOOK",
	"MICROSOFT_TEAMS",
	"PROMETHEUS",
}

var deprecatedIntegrationTypes = []string{
	"NEW_RELIC",
	"FLOWDOCK",
}

var requiredPerType = map[string][]string{
	"PAGER_DUTY":      {"service_key"},
	"DATADOG":         {"api_key", "region"},
	"OPS_GENIE":       {"api_key", "region"},
	"VICTOR_OPS":      {"api_key"},
	"WEBHOOK":         {"url"},
	"MICROSOFT_TEAMS": {"microsoft_teams_webhook_url"},
	"PROMETHEUS":      {"user_name", "password", "service_discovery", "scheme", "enabled"},
}

func resourceMongoDBAtlasThirdPartyIntegration() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceMongoDBAtlasThirdPartyIntegrationCreate,
		ReadContext:   resourceMongoDBAtlasThirdPartyIntegrationRead,
		UpdateContext: resourceMongoDBAtlasThirdPartyIntegrationUpdate,
		DeleteContext: resourceMongoDBAtlasThirdPartyIntegrationDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceMongoDBAtlasThirdPartyIntegrationImportState,
		},
		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"type": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				ValidateDiagFunc: validateIntegrationType(),
			},
			"api_key": {
				Type:      schema.TypeString,
				Sensitive: true,
				Optional:  true,
			},
			"region": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"service_key": {
				Type:      schema.TypeString,
				Sensitive: true,
				Optional:  true,
			},
			"team_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"channel_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"routing_key": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},
			"url": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"secret": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},
			"microsoft_teams_webhook_url": {
				Type:      schema.TypeString,
				Sensitive: true,
				Optional:  true,
			},
			"user_name": {
				Type:      schema.TypeString,
				Sensitive: true,
				Optional:  true,
			},
			"password": {
				Type:      schema.TypeString,
				Sensitive: true,
				Optional:  true,
			},
			"service_discovery": {
				Type:      schema.TypeString,
				Sensitive: true,
				Optional:  true,
			},
			"scheme": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
			},
		},
	}
}

func resourceMongoDBAtlasThirdPartyIntegrationCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*MongoDBClient).Atlas
	projectID := d.Get("project_id").(string)
	integrationType := d.Get("type").(string)

	// checking per type fields
	if requiredSet, ok := requiredPerType[integrationType]; ok {
		for _, key := range requiredSet {
			_, valid := d.GetOk(key)

			if !valid {
				return diag.FromErr(fmt.Errorf("error attribute for third party integration %s. please set it", key))
			}
		}
	}

	requestBody := schemaToIntegration(d)

	_, _, err := conn.Integrations.Create(ctx, projectID, integrationType, requestBody)

	if err != nil {
		return diag.FromErr(fmt.Errorf("error creating third party integration %s", err))
	}

	// ID is equal to project_id+type need to ask
	d.SetId(encodeStateID(map[string]string{
		"project_id": projectID,
		"type":       integrationType,
	}))

	return resourceMongoDBAtlasThirdPartyIntegrationRead(ctx, d, meta)
}

func resourceMongoDBAtlasThirdPartyIntegrationRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*MongoDBClient).Atlas
	ids := decodeStateID(d.Id())

	projectID := ids["project_id"]
	integrationType := ids["type"]

	integration, resp, err := conn.Integrations.Get(context.Background(), projectID, integrationType)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			d.SetId("")
			return nil
		}

		return diag.FromErr(fmt.Errorf("error getting third party integration resource info %s %w", integrationType, err))
	}

	integrationMap := integrationToSchema(d, integration)

	for key, val := range integrationMap {
		if err := d.Set(key, val); err != nil {
			return diag.FromErr(fmt.Errorf("error setting `%s` for third party integration (%s): %s", key, d.Id(), err))
		}
	}

	d.SetId(encodeStateID(map[string]string{
		"project_id": projectID,
		"type":       integrationType,
	}))

	return nil
}

func resourceMongoDBAtlasThirdPartyIntegrationUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*MongoDBClient).Atlas
	ids := decodeStateID(d.Id())

	projectID := ids["project_id"]
	integrationType := ids["type"]

	integration, _, err := conn.Integrations.Get(ctx, projectID, integrationType)

	if err != nil {
		return diag.FromErr(fmt.Errorf("error getting third party integration resource info %s %w", integrationType, err))
	}

	// check for changed attributes per type

	updateIntegrationFromSchema(d, integration)

	_, _, err = conn.Integrations.Replace(ctx, projectID, integrationType, integration)

	if err != nil {
		return diag.FromErr(fmt.Errorf("error updating third party integration type `%s` (%s): %w", integrationType, d.Id(), err))
	}

	return resourceMongoDBAtlasThirdPartyIntegrationRead(ctx, d, meta)
}

func resourceMongoDBAtlasThirdPartyIntegrationDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*MongoDBClient).Atlas
	ids := decodeStateID(d.Id())

	projectID := ids["project_id"]
	integrationType := ids["type"]

	_, err := conn.Integrations.Delete(ctx, projectID, integrationType)

	if err != nil {
		return diag.FromErr(fmt.Errorf("error deleting third party integration type `%s` (%s): %w", integrationType, d.Id(), err))
	}

	return nil
}

func resourceMongoDBAtlasThirdPartyIntegrationImportState(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	conn := meta.(*MongoDBClient).Atlas

	projectID, integrationType, err := splitIntegrationTypeID(d.Id())

	if err != nil {
		return nil, err
	}

	integration, _, err := conn.Integrations.Get(ctx, projectID, integrationType)

	if err != nil {
		return nil, fmt.Errorf("couldn't import third party integration (%s) in project(%s), error: %w", integrationType, projectID, err)
	}

	if err := d.Set("project_id", projectID); err != nil {
		return nil, fmt.Errorf("error setting `project_id` for third party integration (%s): %w", d.Id(), err)
	}

	if err := d.Set("type", integration.Type); err != nil {
		return nil, fmt.Errorf("error setting `type` for third party integration (%s): %w", d.Id(), err)
	}

	d.SetId(encodeStateID(map[string]string{
		"project_id": projectID,
		"type":       integrationType,
	}))

	return []*schema.ResourceData{d}, nil
}

// format {project_id}-{integration_type}
func splitIntegrationTypeID(id string) (projectID, integrationType string, err error) {
	var re = regexp.MustCompile(`(?s)^([0-9a-fA-F]{24})-(.*)$`)
	parts := re.FindStringSubmatch(id)

	if len(parts) != 3 {
		err = fmt.Errorf("import format error: to import a third party integration, use the format {project_id}-{integration_type} %s, %+v", id, parts)
		return
	}

	projectID, integrationType = parts[1], parts[2]

	return
}

func validateIntegrationType() schema.SchemaValidateDiagFunc {
	return func(v any, p cty.Path) diag.Diagnostics {
		value := v.(string)
		var diags diag.Diagnostics
		if !isElementExist(integrationTypes, value) {
			diagError := diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Invalid Third Party Integration type",
				Detail:   fmt.Sprintf("Third Party integration type %q is not a valid value. Possible values are: %q.", value, integrationTypes),
			}
			diags = append(diags, diagError)
		}
		if isElementExist(deprecatedIntegrationTypes, value) {
			diagWarn := diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  "Warning deprecated Third Party Integration type",
				Detail:   fmt.Sprintf("Third Party integration type %q is a deprecated value. This field type values %q are deprecated and will be removed in 1.9.0 release", value, deprecatedIntegrationTypes),
			}
			diags = append(diags, diagWarn)
		}
		return diags
	}
}

func isElementExist(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}
