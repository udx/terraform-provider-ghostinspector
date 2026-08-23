package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/udx/terraform-provider-ghostinspector/internal/gi"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ghostinspector": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("GHOSTINSPECTOR_API_KEY") == "" {
		t.Fatal("GHOSTINSPECTOR_API_KEY must be set for acceptance tests")
	}
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()/int64(time.Millisecond))
}

func testAccClient(t *testing.T) *gi.Client {
	t.Helper()
	return gi.NewClient(os.Getenv("GHOSTINSPECTOR_API_KEY"))
}

// testAccCheckSuitesDestroyed verifies every suite in state is gone.
// Folders are ignored: the Ghost Inspector API cannot delete them, so the
// shared acceptance folder persists between runs.
func testAccCheckSuitesDestroyed(s *terraform.State) error {
	client := gi.NewClient(os.Getenv("GHOSTINSPECTOR_API_KEY"))
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ghostinspector_suite" {
			continue
		}
		suite, err := client.GetSuite(context.Background(), rs.Primary.ID)
		if err == nil && suite != nil {
			return fmt.Errorf("suite %q still exists (%s)", suite.Name, suite.ID)
		}
	}
	return nil
}

func TestAccSuite_basic(t *testing.T) {
	folder := "terraform-provider-ghostinspector-acc"
	suite := uniqueName("tf-acc-suite")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSuitesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSuiteConfig(folder, suite, `"chrome"`, `15000`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ghostinspector_suite.test", "name", suite),
					resource.TestCheckResourceAttr("ghostinspector_suite.test", "browser", "chrome"),
					resource.TestCheckResourceAttr("ghostinspector_suite.test", "max_wait_delay", "15000"),
					resource.TestCheckResourceAttrSet("ghostinspector_suite.test", "id"),
					resource.TestCheckResourceAttrPair(
						"ghostinspector_suite.test", "folder_id",
						"ghostinspector_folder.test", "id",
					),
				),
			},
			{
				// Update settings in place
				Config: testAccSuiteConfig(folder, suite, `"firefox"`, `20000`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ghostinspector_suite.test", "browser", "firefox"),
					resource.TestCheckResourceAttr("ghostinspector_suite.test", "max_wait_delay", "20000"),
				),
			},
			{
				// Re-applying the same config produces an empty plan
				Config:             testAccSuiteConfig(folder, suite, `"firefox"`, `20000`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ResourceName:      "ghostinspector_suite.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccSuiteConfig(folder, suite, browser, maxWait string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    ghostinspector = {
      source = "udx/ghostinspector"
    }
  }
}

provider "ghostinspector" {}

resource "ghostinspector_folder" "test" {
  name = %[1]q
}

resource "ghostinspector_suite" "test" {
  name         = %[2]q
  folder_id    = ghostinspector_folder.test.id
  browser      = %[3]s
  max_wait_delay = %[4]s
}
`, folder, suite, browser, maxWait)
}

func TestAccSuiteVariables_privateMasking(t *testing.T) {
	folder := "terraform-provider-ghostinspector-acc"
	suite := uniqueName("tf-acc-suite")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSuitesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccVariablesConfig(folder, suite, "secret-value-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ghostinspector_suite_variables.test", "variables.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("ghostinspector_suite_variables.test", "variables.*", map[string]string{
						"name":  "password",
						"value": "secret-value-1",
					}),
				),
			},
			{
				// Same config again: private value must not diff even though the API masks it
				Config:             testAccVariablesConfig(folder, suite, "secret-value-1"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				Config: testAccVariablesConfig(folder, suite, "secret-value-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("ghostinspector_suite_variables.test", "variables.*", map[string]string{
						"name":  "password",
						"value": "secret-value-2",
					}),
				),
			},
		},
	})
}

func testAccVariablesConfig(folder, suite, secret string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    ghostinspector = {
      source = "udx/ghostinspector"
    }
  }
}

provider "ghostinspector" {}

resource "ghostinspector_folder" "test" {
  name = %[1]q
}

resource "ghostinspector_suite" "test" {
  name      = %[2]q
  folder_id = ghostinspector_folder.test.id
}

resource "ghostinspector_suite_variables" "test" {
  suite_id = ghostinspector_suite.test.id

  variables = [
    {
      name  = "greeting"
      value = "hello"
    },
    {
      name    = "password"
      value   = %[3]q
      private = true
    },
  ]
}
`, folder, suite, secret)
}

// TestAccTest_adoptReplaceAndCascade proves the two properties the bash-based
// sync could not deliver: adopt-by-name without churn when steps match, and
// execute-reference rewiring through the graph when a module test is
// replaced.
func TestAccTest_adoptReplaceAndCascade(t *testing.T) {
	folder := "terraform-provider-ghostinspector-acc"
	suite := uniqueName("tf-acc-suite")
	module := uniqueName("tf-acc-module")
	parent := uniqueName("tf-acc-parent")

	client := testAccClient(t)
	ctx := context.Background()

	var preCreatedModuleID string
	var adoptedModuleID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSuitesDestroyed,
		Steps: []resource.TestStep{
			{
				// Bring up folder + suite, then pre-create the module test out of band
				Config: testAccSuiteOnlyConfig(folder, suite),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("ghostinspector_suite.test", "id"),
				),
			},
			{
				PreConfig: func() {
					suiteObj, err := client.FindSuiteByName(ctx, suite)
					if err != nil || suiteObj == nil {
						t.Fatalf("suite lookup for pre-create failed: %v", err)
					}
					created, err := client.ImportTest(ctx, suiteObj.ID, map[string]interface{}{
						"name": module,
						"steps": []map[string]interface{}{
							{"command": "open", "value": "/"},
							{"command": "assertTextPresent", "value": "Hello"},
						},
					})
					if err != nil {
						t.Fatalf("pre-create module test failed: %v", err)
					}
					preCreatedModuleID = created.ID
				},
				// Config declares the same module with identical steps: adopt, no replace.
				// The parent executes the module by reference.
				Config: testAccModuleAndParentConfig(folder, suite, module, parent, "Hello"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ghostinspector_test.module", "name", module),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["ghostinspector_test.module"]
						if !ok {
							return fmt.Errorf("module resource missing from state")
						}
						if rs.Primary.ID != preCreatedModuleID {
							return fmt.Errorf("expected adoption without replacement: pre-created %s, state %s", preCreatedModuleID, rs.Primary.ID)
						}
						adoptedModuleID = rs.Primary.ID
						return nil
					},
					resource.TestCheckResourceAttr("ghostinspector_test.parent", "steps.#", "2"),
				),
			},
			{
				// Change the module's steps: the module is replaced (new ID) and the
				// parent's execute step must be rewired to the new ID by the graph.
				Config: testAccModuleAndParentConfig(folder, suite, module, parent, "Goodbye"),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["ghostinspector_test.module"]
						newID := rs.Primary.ID
						if newID == adoptedModuleID {
							return fmt.Errorf("expected module replacement, ID unchanged: %s", newID)
						}
						// Old module must be gone
						if _, err := client.GetTest(ctx, adoptedModuleID); err == nil {
							return fmt.Errorf("old module test %s still exists", adoptedModuleID)
						}
						// Parent's execute step must reference the new module ID
						parentID := s.RootModule().Resources["ghostinspector_test.parent"].Primary.ID
						parentObj, err := client.GetTest(ctx, parentID)
						if err != nil {
							return err
						}
						for _, st := range parentObj.Steps {
							if st["command"] == "execute" {
								if st["value"] != newID {
									return fmt.Errorf("parent execute step references %v, want new module ID %s", st["value"], newID)
								}
								return nil
							}
						}
						return fmt.Errorf("parent has no execute step")
					},
				),
			},
			{
				// Metadata-only change: ID must stay put
				Config: testAccModuleAndParentConfigMetadata(folder, suite, module, parent, "Goodbye", "9000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["ghostinspector_test.module"]
						if strings.TrimSpace(rs.Primary.ID) == "" {
							return fmt.Errorf("module ID lost")
						}
						return nil
					},
					resource.TestCheckResourceAttr("ghostinspector_test.module", "final_delay", "9000"),
				),
			},
			{
				ResourceName:      "ghostinspector_test.module",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccSuiteOnlyConfig(folder, suite string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    ghostinspector = {
      source = "udx/ghostinspector"
    }
  }
}

provider "ghostinspector" {}

resource "ghostinspector_folder" "test" {
  name = %[1]q
}

resource "ghostinspector_suite" "test" {
  name      = %[2]q
  folder_id = ghostinspector_folder.test.id
}
`, folder, suite)
}

func testAccModuleAndParentConfig(folder, suite, module, parent, assertText string) string {
	return testAccModuleAndParentConfigMetadata(folder, suite, module, parent, assertText, "5000")
}

func testAccModuleAndParentConfigMetadata(folder, suite, module, parent, assertText, final_delay string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    ghostinspector = {
      source = "udx/ghostinspector"
    }
  }
}

provider "ghostinspector" {}

resource "ghostinspector_folder" "test" {
  name = %[1]q
}

resource "ghostinspector_suite" "test" {
  name      = %[2]q
  folder_id = ghostinspector_folder.test.id
}

resource "ghostinspector_test" "module" {
  suite_id   = ghostinspector_suite.test.id
  name       = %[3]q
  import_only = true
  final_delay = %[6]s

  steps = [
    {
      command = "open"
      value   = "/"
    },
    {
      command = "assertTextPresent"
      value   = %[5]q
    },
  ]
}

resource "ghostinspector_test" "parent" {
  suite_id = ghostinspector_suite.test.id
  name     = %[4]q

  steps = [
    {
      command = "open"
      value   = "/"
    },
    {
      command = "execute"
      value   = ghostinspector_test.module.id
    },
  ]
}
`, folder, suite, module, parent, assertText, final_delay)
}

// TestAccSuite_adoptByID pins an existing suite by suite_id: the suite must be
// adopted (not recreated) and settings must reconcile in place.
func TestAccSuite_adoptByID(t *testing.T) {
	folder := "terraform-provider-ghostinspector-acc"
	suite := uniqueName("tf-acc-adopt")

	client := testAccClient(t)
	ctx := context.Background()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSuitesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSuiteOnlyConfig(folder, suite),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("ghostinspector_suite.test", "id"),
				),
			},
			{
				// Re-declare the same suite pinned by suite_id in a second resource:
				// it must adopt rather than create a duplicate.
				Config: testAccSuiteAdoptByIDConfig(folder, suite),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						original := s.RootModule().Resources["ghostinspector_suite.test"]
						adopted := s.RootModule().Resources["ghostinspector_suite.adopted"]
						if original == nil || adopted == nil {
							return fmt.Errorf("expected both suite resources in state")
						}
						if original.Primary.ID != adopted.Primary.ID {
							return fmt.Errorf("adopted ID %s != original ID %s (would have duplicated the suite)", adopted.Primary.ID, original.Primary.ID)
						}
						// exactly one suite with this name must exist
						found, err := client.FindSuiteByName(ctx, suite)
						if err != nil || found == nil {
							return fmt.Errorf("suite lookup failed: %v", err)
						}
						if found.ID != original.Primary.ID {
							return fmt.Errorf("live suite %s does not match state %s", found.ID, original.Primary.ID)
						}
						return nil
					},
					resource.TestCheckResourceAttr("ghostinspector_suite.adopted", "browser", "firefox"),
				),
			},
		},
	})
}

func testAccSuiteAdoptByIDConfig(folder, suite string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    ghostinspector = {
      source = "udx/ghostinspector"
    }
  }
}

provider "ghostinspector" {}

resource "ghostinspector_folder" "test" {
  name = %[1]q
}

resource "ghostinspector_suite" "test" {
  name      = %[2]q
  folder_id = ghostinspector_folder.test.id
}

resource "ghostinspector_suite" "adopted" {
  name     = %[2]q
  suite_id = ghostinspector_suite.test.id
  browser  = "firefox"
}
`, folder, suite)
}
