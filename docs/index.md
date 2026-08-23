# Ghost Inspector Provider

Manage Ghost Inspector suites, tests, steps, and suite variables as code. Works with Terraform and OpenTofu.

```hcl
provider "ghostinspector" {
  api_key = var.ghostinspector_api_key # or GHOSTINSPECTOR_API_KEY env var
}
```

## Resources

- [ghostinspector_folder](resources/folder.md) - folder (adopts by name; API cannot delete folders)
- [ghostinspector_suite](resources/suite.md) - suite, default test settings, schedule
- [ghostinspector_suite_variables](resources/suite_variables.md) - complete variable set of a suite
- [ghostinspector_test](resources/test.md) - one test including its steps

## Data sources

- [ghostinspector_folder](data-sources/folder.md) - folder lookup by id or name

## Key semantics

- Null settings attributes are unmanaged and mirror the API-side value.
- Step changes replace the test (the API cannot update steps in place): export, delete, re-import, new test ID, history stays with the removed test.
- Resources adopt existing same-named objects on create instead of duplicating them.
- Private variable values are write-only (the API masks them); the configured value is kept in state.
