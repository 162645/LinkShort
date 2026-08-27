package domain

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type URLServiceTestSuite struct {
	suite.Suite
	service *URLService
	db      *database.PostgreSQL
	cache   *cache.Redis
}

func (suite *URLServiceTestSuite) SetupSuite() {
	if !canConnect("127.0.0.1:5432") || !canConnect("127.0.0.1:6379") {
		suite.T().Skip("url service integration tests skipped: postgres/redis dependencies are unavailable")
	}

	os.Setenv("DATABASE_URL", "postgres://postgres:password@localhost:5432/url_shortener_db?sslmode=disable")
	os.Setenv("REDIS_URL", "redis://:redispassword@localhost:6379/1")

	suite.db = database.NewPostgreSQL()
	suite.cache = cache.NewRedis()

	ctx := context.Background()
	suite.db.Pool.Exec(ctx, "DROP TABLE IF EXISTS click_events CASCADE")
	suite.db.Pool.Exec(ctx, "DROP TABLE IF EXISTS url_mappings CASCADE")

	err := suite.db.AutoMigrate()
	assert.NoError(suite.T(), err)

	suite.service = NewURLService(suite.db, suite.cache)
}

func (suite *URLServiceTestSuite) TearDownSuite() {
	if suite.cache != nil {
		suite.cache.FlushDB()
		suite.cache.Close()
	}
	if suite.db != nil {
		suite.db.Close()
	}
}

func (suite *URLServiceTestSuite) SetupTest() {
	ctx := context.Background()
	suite.db.Pool.Exec(ctx, "TRUNCATE TABLE url_mappings, click_events CASCADE")
	suite.cache.FlushDB()
}

func (suite *URLServiceTestSuite) TestShortenURL() {
	// 修复：传入 context.TODO()
	ctx := context.TODO()
	req := &CreateURLRequest{
		LongURL: "https://example.com/very/long/url/path",
		UserID:  "test_user_123",
		Metadata: map[string]string{
			"source":   "api",
			"campaign": "test",
		},
	}

	url, err := suite.service.ShortenURL(ctx, req) // 这里补了 ctx
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), url)
	assert.NotEmpty(suite.T(), url.ShortCode)
	assert.Equal(suite.T(), req.LongURL, url.LongURL)
	assert.Equal(suite.T(), req.UserID, url.UserID)
	assert.True(suite.T(), url.IsActive)
	assert.NotZero(suite.T(), url.ID)
	assert.False(suite.T(), url.CreatedAt.IsZero())
}

func (suite *URLServiceTestSuite) TestShortenURLWithCustomAlias() {
	ctx := context.TODO()
	req := &CreateURLRequest{
		LongURL:     "https://example.com/custom",
		CustomAlias: "mycustom",
		UserID:      "test_user_123",
	}

	url, err := suite.service.ShortenURL(ctx, req) // 这里补了 ctx
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "mycustom", url.ShortCode)
}

func (suite *URLServiceTestSuite) TestShortenURLWithExpiration() {
	ctx := context.TODO()
	expirationTime := time.Now().Add(24 * time.Hour)
	req := &CreateURLRequest{
		LongURL:        "https://example.com/expires",
		UserID:         "test_user_123",
		ExpirationTime: &expirationTime,
	}

	url, err := suite.service.ShortenURL(ctx, req) // 这里补了 ctx
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), url.ExpiresAt)
	assert.WithinDuration(suite.T(), expirationTime, *url.ExpiresAt, time.Second)
}

func (suite *URLServiceTestSuite) TestGetURL() {
	ctx := context.TODO()
	req := &CreateURLRequest{
		LongURL: "https://example.com/get-test",
		UserID:  "test_user_123",
	}

	createdURL, err := suite.service.ShortenURL(ctx, req) // 这里补了 ctx
	assert.NoError(suite.T(), err)

	// 修复：补了 ctx
	retrievedURL, err := suite.service.GetURL(ctx, createdURL.ShortCode, "test_user_123")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), createdURL.ShortCode, retrievedURL.ShortCode)
	assert.Equal(suite.T(), createdURL.LongURL, retrievedURL.LongURL)
	assert.Equal(suite.T(), createdURL.UserID, retrievedURL.UserID)
}

func (suite *URLServiceTestSuite) TestGetURLUnauthorized() {
	ctx := context.TODO()
	req := &CreateURLRequest{
		LongURL: "https://example.com/unauthorized-test",
		UserID:  "test_user_123",
	}

	createdURL, err := suite.service.ShortenURL(ctx, req) // 这里补了 ctx
	assert.NoError(suite.T(), err)

	// 修复：补了 ctx
	_, err = suite.service.GetURL(ctx, createdURL.ShortCode, "wrong_user")
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrUnauthorized, err)
}

func (suite *URLServiceTestSuite) TestDeleteURL() {
	ctx := context.TODO()
	req := &CreateURLRequest{
		LongURL: "https://example.com/delete-test",
		UserID:  "test_user_123",
	}

	createdURL, err := suite.service.ShortenURL(ctx, req) // 这里补了 ctx
	assert.NoError(suite.T(), err)

	// 修复：补了 ctx
	err = suite.service.DeleteURL(ctx, createdURL.ShortCode, "test_user_123")
	assert.NoError(suite.T(), err)

	// 修复：补了 ctx
	_, err = suite.service.GetURL(ctx, createdURL.ShortCode, "test_user_123")
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrURLNotFound, err)
}

func (suite *URLServiceTestSuite) TestValidateURL() {
	invalidURLs := []string{"", "not-a-url", "ftp://example.com", "http://", "https://"}
	for _, invalidURL := range invalidURLs {
		err := suite.service.validateURL(invalidURL)
		assert.Error(suite.T(), err)
		assert.Equal(suite.T(), ErrInvalidURL, err)
	}
}

func (suite *URLServiceTestSuite) TestValidateShortCode() {
	invalidCodes := []string{"", "ab", "abcdefghijk", "abc-def"}
	for _, invalidCode := range invalidCodes {
		err := suite.service.validateShortCode(invalidCode)
		assert.Error(suite.T(), err)
		assert.Equal(suite.T(), ErrInvalidShortCode, err)
	}
}

func TestURLServiceTestSuite(t *testing.T) {
	suite.Run(t, new(URLServiceTestSuite))
}

func canConnect(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 600*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
