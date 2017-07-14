package azurerm

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/arm/mysql"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/jen20/riviera/azure"
)

func resourceArmMySqlServer() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmMySqlServerCreate,
		Read:   resourceArmMySqlServerRead,
		Update: resourceArmMySqlServerUpdate,
		Delete: resourceArmMySqlServerDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": locationSchema(),

			"resource_group_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"sku": {
				Type:     schema.TypeSet,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
							ValidateFunc: validation.StringInSlice([]string{
								"MYSQLB50",
								"MYSQLB100",
							}, true),
							DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
						},

						"capacity": {
							Type:     schema.TypeInt,
							Required: true,
							ValidateFunc: validateIntInSlice([]int{
								50,
								100,
							}),
						},

						"tier": {
							Type:     schema.TypeString,
							Required: true,
							ValidateFunc: validation.StringInSlice([]string{
								string(mysql.Basic),
							}, true),
							DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
						},
					},
				},
			},

			"administrator_login": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"administrator_login_password": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
			},

			"version": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(mysql.FiveFullStopSix),
					string(mysql.FiveFullStopSeven),
				}, true),
				DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
				ForceNew:         true,
			},

			"storage_mb": {
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
				ValidateFunc: validateIntInSlice([]int{
					51200,
					102400,
				}),
			},

			"ssl_enforcement": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(mysql.SslEnforcementEnumDisabled),
					string(mysql.SslEnforcementEnumEnabled),
				}, true),
				DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
			},

			"fqdn": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceArmMySqlServerCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).mysqlServersClient

	log.Printf("[INFO] preparing arguments for AzureRM MySQL Server creation.")

	name := d.Get("name").(string)
	location := d.Get("location").(string)
	resGroup := d.Get("resource_group_name").(string)

	// TODO: fix me
	//adminLogin := d.Get("administrator_login").(string)
	//adminLoginPassword := d.Get("administrator_login_password").(string)
	sslEnforcement := d.Get("ssl_enforcement").(string)
	version := d.Get("version").(string)
	storageMB := d.Get("storage_mb").(int)

	tags := d.Get("tags").(map[string]interface{})

	sku := expandMySQLServerSku(d, storageMB)

	properties := mysql.ServerForCreate{
		Location: &location,
		Sku:      sku,
		Properties: &mysql.ServerPropertiesForCreate{
			Version:        mysql.ServerVersion(version),
			StorageMB:      azure.Int64(int64(storageMB)),
			SslEnforcement: mysql.SslEnforcementEnum(sslEnforcement),
			// TODO: fix the generator
			//AdministratorLogin: azure.String(adminLogin),
			//AdministratorLoginPassword: azure.String(adminLoginPassword),
		},
		Tags: expandTags(tags),
	}

	_, error := client.CreateOrUpdate(resGroup, name, properties, make(chan struct{}))
	err := <-error
	if err != nil {
		return err
	}

	read, err := client.Get(resGroup, name)
	if err != nil {
		return err
	}
	if read.ID == nil {
		return fmt.Errorf("Cannot read MySQL Server %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*read.ID)

	return resourceArmMySqlServerRead(d, meta)
}

func resourceArmMySqlServerUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).mysqlServersClient

	log.Printf("[INFO] preparing arguments for AzureRM MySQL Server update.")

	name := d.Get("name").(string)
	resGroup := d.Get("resource_group_name").(string)

	adminLoginPassword := d.Get("administrator_login_password").(string)
	sslEnforcement := d.Get("ssl_enforcement").(string)
	version := d.Get("version").(string)
	storageMB := d.Get("storage_mb").(int)
	sku := expandMySQLServerSku(d, storageMB)

	tags := d.Get("tags").(map[string]interface{})

	properties := mysql.ServerUpdateParameters{
		Sku: sku,
		ServerUpdateParametersProperties: &mysql.ServerUpdateParametersProperties{
			SslEnforcement:             mysql.SslEnforcementEnum(sslEnforcement),
			StorageMB:                  azure.Int64(int64(storageMB)),
			Version:                    mysql.ServerVersion(version),
			AdministratorLoginPassword: azure.String(adminLoginPassword),
		},
		Tags: expandTags(tags),
	}

	_, error := client.Update(resGroup, name, properties, make(chan struct{}))
	err := <-error
	if err != nil {
		return err
	}

	read, err := client.Get(resGroup, name)
	if err != nil {
		return err
	}
	if read.ID == nil {
		return fmt.Errorf("Cannot read MySQL Server %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*read.ID)

	return resourceArmMySqlServerRead(d, meta)
}

func resourceArmMySqlServerRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).mysqlServersClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	name := id.Path["servers"]

	resp, err := client.Get(resGroup, name)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error making Read request on Azure MySQL Server %s: %+v", name, err)
	}

	d.Set("name", resp.Name)
	d.Set("resource_group_name", resGroup)
	d.Set("location", azureRMNormalizeLocation(*resp.Location))

	d.Set("administrator_login", resp.AdministratorLogin)
	d.Set("version", string(resp.Version))
	d.Set("storage_mb", int(*resp.StorageMB))
	d.Set("ssl_enforcement", string(resp.SslEnforcement))

	flattenAndSetMySQLServerSku(d, resp.Sku)
	flattenAndSetTags(d, resp.Tags)

	// Computed
	d.Set("fqdn", resp.FullyQualifiedDomainName)

	return nil
}

func resourceArmMySqlServerDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).mysqlServersClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	name := id.Path["servers"]

	_, error := client.Delete(resGroup, name, make(chan struct{}))
	err = <-error

	return err
}

func expandMySQLServerSku(d *schema.ResourceData, storageMB int) *mysql.Sku {
	skus := d.Get("sku").(*schema.Set).List()
	sku := skus[0].(map[string]interface{})

	name := sku["name"].(string)
	capacity := sku["capacity"].(int)
	tier := sku["tier"].(string)

	return &mysql.Sku{
		Name:     azure.String(name),
		Capacity: azure.Int32(int32(capacity)),
		Tier:     mysql.SkuTier(tier),
		Size:     azure.String(strconv.Itoa(storageMB)),
	}
}

func flattenAndSetMySQLServerSku(d *schema.ResourceData, resp *mysql.Sku) {
	values := map[string]interface{}{}

	values["name"] = *resp.Name
	values["capacity"] = int(*resp.Capacity)
	values["tier"] = string(resp.Tier)

	sku := []interface{}{values}
	d.Set("sku", sku)
}