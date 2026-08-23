terraform {
  required_providers {
    ghostinspector = {
      source = "udx/ghostinspector"
    }
  }
}

# export GHOSTINSPECTOR_API_KEY instead of passing the key inline
provider "ghostinspector" {}

resource "ghostinspector_folder" "app" {
  name = "My App"
}

resource "ghostinspector_suite" "release" {
  name      = "Automated Tests"
  folder_id = ghostinspector_folder.app.id
  browser   = "chrome"
  region    = "us-east-1"

  schedule = {
    enabled  = true
    interval = "daily"
    time     = "08:00"
  }
}

resource "ghostinspector_suite_variables" "release" {
  suite_id = ghostinspector_suite.release.id

  variables = [
    { name = "siteUrl", value = "https://staging.example.com" },
    { name = "defaultUser", value = "gi-test@example.com" },
    { name = "defaultUserPassword", value = var.test_password, private = true },
  ]
}

variable "test_password" {
  type      = string
  sensitive = true
}

resource "ghostinspector_test" "login_module" {
  suite_id    = ghostinspector_suite.release.id
  name        = "Login User"
  import_only = true

  steps = [
    { command = "open", value = "/login" },
    { command = "assign", target = "input[name=email]", value = "{{defaultUser}}" },
    { command = "assign", target = "input[name=password]", value = "{{defaultUserPassword}}", private = true },
    { command = "click", target = "button[type=submit]" },
    { command = "pause", value = "2000" },
  ]
}

resource "ghostinspector_test" "account_page" {
  suite_id = ghostinspector_suite.release.id
  name     = "Account page (logged in)"

  steps = [
    { command = "execute", value = ghostinspector_test.login_module.id },
    { command = "open", value = "/account" },
    { command = "assertElementPresent", target = ".account-dashboard" },
    { command = "screenshot" },
  ]
}
