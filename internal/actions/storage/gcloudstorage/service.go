package gcloudstorage

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "strings"

    "cloud.google.com/go/compute/metadata"
    "cloud.google.com/go/storage"
    "golang.org/x/oauth2/google"
    "google.golang.org/api/iterator"
)

type gcsService struct {
	client *storage.Client
	creds  *google.Credentials
}

func newGCSService(ctx context.Context) (Service, error) {
	creds, err := google.FindDefaultCredentials(ctx, storage.ScopeReadWrite)
	if err != nil {
		return nil, err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return &gcsService{client: client, creds: creds}, nil
}

func (s *gcsService) Close() error {
	return s.client.Close()
}

func (s *gcsService) ObjectExists(ctx context.Context, path StoragePath) (bool, error) {
	if path.Object == "" {
		_, err := s.client.Bucket(path.Bucket).Attrs(ctx)
		if errors.Is(err, storage.ErrBucketNotExist) {
			return false, nil
		}
		return err == nil, err
	}
	_, err := s.client.Bucket(path.Bucket).Object(path.Object).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *gcsService) CopyObject(ctx context.Context, src, dst StoragePath) error {
	copier := s.client.Bucket(dst.Bucket).Object(dst.Object).CopierFrom(s.client.Bucket(src.Bucket).Object(src.Object))
	_, err := copier.Run(ctx)
	return err
}

func (s *gcsService) DeleteObject(ctx context.Context, path StoragePath) error {
	return s.client.Bucket(path.Bucket).Object(path.Object).Delete(ctx)
}

func (s *gcsService) List(ctx context.Context, path StoragePath, recursive bool) (ServiceListResult, error) {
    bucket := s.client.Bucket(path.Bucket)

    prefix := path.Object
    if prefix == "" {
        prefix = ""
    }

    result := ServiceListResult{}

	query := &storage.Query{Prefix: prefix}
	if !recursive {
		query.Delimiter = "/"
	}

	it := bucket.Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return ServiceListResult{}, err
		}
		if attrs == nil {
			continue
		}
		if attrs.Prefix != "" {
			result.Prefixes = append(result.Prefixes, attrs.Prefix)
			continue
		}
		result.Objects = append(result.Objects, ObjectAttrs{
			Name:         attrs.Name,
			Size:         attrs.Size,
			Updated:      attrs.Updated,
			ContentType:  attrs.ContentType,
			StorageClass: attrs.StorageClass,
		})
	}

    if prefix == "" {
        // We cannot reliably check bucket existence without buckets.get.
        // Keep behavior consistent and mark as exists.
        result.Exists = true
    } else {
        result.Exists = len(result.Objects) > 0 || len(result.Prefixes) > 0
    }

    return result, nil
}

func (s *gcsService) FetchAuthInfo(ctx context.Context) (AuthInfo, error) {
    info := AuthInfo{}
    if s.creds != nil {
        info.ProjectID = s.creds.ProjectID
        if len(s.creds.JSON) > 0 {
            account, err := extractAccountFromJSON(s.creds.JSON)
            if err == nil {
                info.Account = account
                info.Source = "credentials_json"
            }
        }
    }

	if info.Account == "" {
		if metadata.OnGCE() {
			if email, err := metadata.Email("default"); err == nil {
				info.Account = email
				info.Source = "metadata"
			}
		}
	}

    if info.ProjectID == "" {
        if metadata.OnGCE() {
            if projectID, err := metadata.ProjectID(); err == nil {
                info.ProjectID = projectID
                if info.Source == "" {
                    info.Source = "metadata"
                }
            }
        }
    }

    // Fallback to common env vars for project id
    if info.ProjectID == "" {
        if v := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT")); v != "" {
            info.ProjectID = v
            if info.Source == "" {
                info.Source = "env"
            }
        }
    }
    if info.ProjectID == "" {
        if v := strings.TrimSpace(os.Getenv("GCLOUD_PROJECT")); v != "" {
            info.ProjectID = v
            if info.Source == "" {
                info.Source = "env"
            }
        }
    }
    if info.ProjectID == "" {
        if v := strings.TrimSpace(os.Getenv("GOOGLE_PROJECT_ID")); v != "" {
            info.ProjectID = v
            if info.Source == "" {
                info.Source = "env"
            }
        }
    }

    if info.ProjectID == "" {
        info.ProjectID = s.creds.ProjectID
    }

    // If account is still empty and we're not on GCE with a default email,
    // try to read the active gcloud account (ADC user flow case).
    if info.Account == "" {
        if acc, err := activeGcloudAccount(ctx); err == nil && strings.TrimSpace(acc) != "" {
            info.Account = strings.TrimSpace(acc)
            if info.Source == "" {
                info.Source = "gcloud"
            }
        }
    }

    return info, nil
}

func extractAccountFromJSON(data []byte) (string, error) {
    type serviceAccount struct {
        ClientEmail string `json:"client_email"`
    }
    var sa serviceAccount
    if err := json.Unmarshal(data, &sa); err != nil {
        return "", err
    }
    return sa.ClientEmail, nil
}

// activeGcloudAccount returns the active gcloud account email, if available.
func activeGcloudAccount(ctx context.Context) (string, error) {
    // Prefer a simple, stable command. On Windows, "gcloud" resolves to gcloud.cmd.
    cmd := exec.CommandContext(ctx, "gcloud", "config", "get-value", "account")
    cmd.Env = os.Environ()
    out, err := cmd.Output()
    if err != nil {
        return "", err
    }
    value := strings.TrimSpace(string(out))
    if value == "" || strings.HasPrefix(strings.ToUpper(value), "ERROR:") {
        return "", fmt.Errorf("no active gcloud account")
    }
    return value, nil
}

var _ Service = (*gcsService)(nil)
