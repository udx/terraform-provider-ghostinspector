# ghostinspector_folder (data source)

Looks up a Ghost Inspector folder by `id` or `name`.

## Example

```hcl
data "ghostinspector_folder" "app" {
  name = "My App"
}

resource "ghostinspector_suite" "release" {
  name      = "Automated Tests"
  folder_id = data.ghostinspector_folder.app.id
}
```

## Argument reference

Exactly one of:

- `id` - Folder ID.
- `name` - Folder name.

## Attribute reference

- `id` - Folder ID.
- `name` - Folder name.
- `organization` - Owning organization ID.
