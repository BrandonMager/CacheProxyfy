package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/BrandonMager/CacheProxyfy/internal/config"
	"github.com/BrandonMager/CacheProxyfy/internal/db"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func bytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}

// infra holds the shared test infrastructure.
type infra struct {
	database *db.DB
	s3Store  *storage.S3
	cfg      *config.Config // pre-wired with local dir + S3 config + DB config
	localDir string
}

// setupInfra starts LocalStack + Postgres and wires a config.Config that
// points run() at the test containers. It also creates a temp dir for local FS.
func setupInfra(t *testing.T) *infra {
	t.Helper()
	ctx := context.Background()

	lsCtr, err := localstack.Run(ctx, "localstack/localstack:3")
	testcontainers.CleanupContainer(t, lsCtr)
	if err != nil {
		t.Fatalf("start localstack: %v", err)
	}

	pgCtr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(pgCtr); err != nil {
			t.Errorf("terminate postgres: %v", err)
		}
	})

	// Resolve LocalStack endpoint.
	mappedPort, err := lsCtr.MappedPort(ctx, "4566/tcp")
	if err != nil {
		t.Fatalf("localstack port: %v", err)
	}
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		t.Fatalf("docker provider: %v", err)
	}
	defer provider.Close()
	host, err := provider.DaemonHost(ctx)
	if err != nil {
		t.Fatalf("daemon host: %v", err)
	}
	endpoint := "http://" + net.JoinHostPort(host, mappedPort.Port())

	// Create S3 bucket.
	const bucket = "migrate-test"
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("aws config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	if _, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	s3Store, err := storage.NewS3(ctx, storage.S3Config{
		Bucket:          bucket,
		Region:          "us-east-1",
		Endpoint:        endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	// Open Postgres.
	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	dbCfg := db.Config{SSLMode: "disable"}
	if _, err := fmt.Sscanf(connStr,
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		&dbCfg.Host, &dbCfg.Port, &dbCfg.User, &dbCfg.Password, &dbCfg.DBName, &dbCfg.SSLMode,
	); err != nil {
		dbCfg = db.Config{Host: "localhost", User: "user", Password: "pass", DBName: "testdb", SSLMode: "disable"}
		fmt.Sscanf(connStr, "postgres://user:pass@localhost:%d/testdb", &dbCfg.Port)
	}

	database, err := db.Open(dbCfg)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	// Temp dir for local FS artifacts.
	localDir, err := os.MkdirTemp("", "migrate-local-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(localDir) })

	// Build a config.Config that wires run() at the test containers.
	cfg := &config.Config{
		Cache: config.CacheConfig{
			Backend:  "local",
			LocalDir: localDir,
		},
		S3: config.S3Config{
			Bucket:          bucket,
			Region:          "us-east-1",
			Endpoint:        endpoint,
			AccessKeyID:     "test",
			SecretAccessKey: "test",
		},
		Database: config.DatabaseConfig{
			Host:     dbCfg.Host,
			Port:     dbCfg.Port,
			User:     dbCfg.User,
			Password: dbCfg.Password,
			DBName:   dbCfg.DBName,
			SSLMode:  dbCfg.SSLMode,
		},
	}

	return &infra{
		database: database,
		s3Store:  s3Store,
		cfg:      cfg,
		localDir: localDir,
	}
}

// TestMigrate_UploadsAllArtifacts verifies the core migration path:
// artifacts on local FS that are recorded in Postgres are all uploaded to S3
// with their contents intact.
func TestMigrate_UploadsAllArtifacts(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	inf := setupInfra(t)

	localStore, err := storage.NewLocal(inf.localDir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	artifacts := []struct {
		pkg     db.Package
		content []byte
	}{
		{
			pkg:     db.Package{Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Checksum: "aabbccdd11223344aabbccdd11223344", SizeBytes: 20},
			content: []byte("lodash tarball bytes"),
		},
		{
			pkg:     db.Package{Ecosystem: "pypi", Name: "requests", Version: "2.31.0", Checksum: "eeff00112233445566778899aabbccdd", SizeBytes: 20},
			content: []byte("requests wheel bytes"),
		},
	}

	for _, a := range artifacts {
		if err := localStore.Put(ctx, a.pkg.Checksum, bytesReader(a.content), int64(len(a.content))); err != nil {
			t.Fatalf("local Put %s: %v", a.pkg.Checksum[:8], err)
		}
		if _, err := inf.database.UpsertPackage(ctx, a.pkg); err != nil {
			t.Fatalf("upsert %s: %v", a.pkg.Name, err)
		}
	}

	if err := run(ctx, inf.cfg, discardLogger(), false, 2); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Every artifact must be in S3 with the correct bytes.
	for _, a := range artifacts {
		rc, err := inf.s3Store.Get(ctx, a.pkg.Checksum)
		if err != nil {
			t.Fatalf("S3 Get %s: %v", a.pkg.Checksum[:8], err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if string(got) != string(a.content) {
			t.Errorf("S3 content for %s: got %q, want %q", a.pkg.Checksum[:8], got, a.content)
		}
	}
}

// TestMigrate_Idempotent verifies that re-running the migrator when an artifact
// is already in S3 skips it without error and leaves the content intact.
func TestMigrate_Idempotent(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	inf := setupInfra(t)

	localStore, err := storage.NewLocal(inf.localDir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	const checksum = "1234567890abcdef1234567890abcdef"
	content := []byte("react tarball bytes")

	if err := localStore.Put(ctx, checksum, bytesReader(content), int64(len(content))); err != nil {
		t.Fatalf("local Put: %v", err)
	}
	if _, err := inf.database.UpsertPackage(ctx, db.Package{
		Ecosystem: "npm", Name: "react", Version: "18.2.0",
		Checksum: checksum, SizeBytes: int64(len(content)),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := run(ctx, inf.cfg, discardLogger(), false, 1); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := run(ctx, inf.cfg, discardLogger(), false, 1); err != nil {
		t.Fatalf("second run (idempotent): %v", err)
	}

	// Content must be intact after both runs.
	rc, err := inf.s3Store.Get(ctx, checksum)
	if err != nil {
		t.Fatalf("S3 Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != string(content) {
		t.Errorf("S3 content after second run: got %q, want %q", got, content)
	}
}

// TestMigrate_MissingLocalArtifact verifies that when a package is recorded in
// Postgres but its artifact is absent from local FS (e.g. already evicted),
// the migrator logs a warning and returns nil — it does not fail the run.
func TestMigrate_MissingLocalArtifact(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	inf := setupInfra(t)

	// Register in DB but do NOT write to local FS.
	if _, err := inf.database.UpsertPackage(ctx, db.Package{
		Ecosystem: "npm", Name: "left-pad", Version: "1.3.0",
		Checksum: "deadbeefdeadbeefdeadbeefdeadbeef", SizeBytes: 512,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := run(ctx, inf.cfg, discardLogger(), false, 1); err != nil {
		t.Fatalf("run should succeed even when a local artifact is missing: %v", err)
	}
}

// TestMigrate_DryRun verifies that --dry-run reports what would be uploaded
// without writing anything to S3.
func TestMigrate_DryRun(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	inf := setupInfra(t)

	localStore, err := storage.NewLocal(inf.localDir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	const checksum = "cafebabecafebabecafebabecafebabe"
	content := []byte("vue tarball bytes")

	if err := localStore.Put(ctx, checksum, bytesReader(content), int64(len(content))); err != nil {
		t.Fatalf("local Put: %v", err)
	}
	if _, err := inf.database.UpsertPackage(ctx, db.Package{
		Ecosystem: "npm", Name: "vue", Version: "3.3.0",
		Checksum: checksum, SizeBytes: int64(len(content)),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := run(ctx, inf.cfg, discardLogger(), true, 1); err != nil {
		t.Fatalf("dry run: %v", err)
	}

	// S3 must still be empty after a dry run.
	exists, err := inf.s3Store.Exists(ctx, checksum)
	if err != nil {
		t.Fatalf("S3 Exists: %v", err)
	}
	if exists {
		t.Error("dry run wrote to S3 — it must not")
	}
}

// TestMigrate_DeduplicatesChecksums verifies that when two package rows share
// the same checksum (content-addressed deduplication), the artifact is uploaded
// to S3 exactly once.
func TestMigrate_DeduplicatesChecksums(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()
	inf := setupInfra(t)

	localStore, err := storage.NewLocal(inf.localDir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	const checksum = "abcdef1234567890abcdef1234567890"
	content := []byte("shared artifact bytes")

	if err := localStore.Put(ctx, checksum, bytesReader(content), int64(len(content))); err != nil {
		t.Fatalf("local Put: %v", err)
	}

	// Two different packages pointing at the same checksum.
	for _, pkg := range []db.Package{
		{Ecosystem: "npm", Name: "pkg-a", Version: "1.0.0", Checksum: checksum, SizeBytes: int64(len(content))},
		{Ecosystem: "npm", Name: "pkg-b", Version: "2.0.0", Checksum: checksum, SizeBytes: int64(len(content))},
	} {
		if _, err := inf.database.UpsertPackage(ctx, pkg); err != nil {
			t.Fatalf("upsert %s: %v", pkg.Name, err)
		}
	}

	if err := run(ctx, inf.cfg, discardLogger(), false, 2); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Artifact must exist in S3 exactly once (S3 is content-addressed, so one key).
	rc, err := inf.s3Store.Get(ctx, checksum)
	if err != nil {
		t.Fatalf("S3 Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != string(content) {
		t.Errorf("S3 content: got %q, want %q", got, content)
	}
}
