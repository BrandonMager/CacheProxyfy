package proxy

import (
	"context"

	"github.com/BrandonMager/CacheProxyfy/internal/db"
)

type CacheClient interface {
	Get(ctx context.Context, ecosystem, name, version string) (string, error)
	Set(ctx context.Context, ecosystem, name, version, checksum string) error
	Ping(ctx context.Context) error
}

type DBClient interface {
	GetPackage(ctx context.Context, ecosystem, name, version string) (db.Package, error)
	TouchPackage(ctx context.Context, ecosystem, name, version string) error
	UpsertPackage(ctx context.Context, pkg db.Package) (string, error)
	RecordEvent(ctx context.Context, ecosystem, name, version, event string, bytes int64) error
}
