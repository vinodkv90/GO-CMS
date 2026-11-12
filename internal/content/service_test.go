package content

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/go-cms/cms/internal/fields"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestContentService(t *testing.T) (*Service, *MemoryContentRepository) {
	repo := NewMemoryContentRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fieldReg := fields.NewRegistry()
	eventBus := NewEventBus()

	svc := NewService(repo, repo, repo, fieldReg, eventBus, logger)
	return svc, repo
}

func TestService_CreateContentTypeAndFields(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()

	// 1. Create Collection Content Type
	ct, err := svc.CreateContentType(ctx, CreateTypeRequest{
		UID:  "article",
		Name: "Articles",
		Kind: KindCollection,
		Fields: []ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
			{Name: "Slug", FieldKey: "slug", FieldType: fields.TypeUID, Configuration: fields.ValidationRules{Required: true}},
			{Name: "Content", FieldKey: "content", FieldType: fields.TypeRichText},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "article", ct.UID)
	assert.Len(t, ct.Fields, 3)

	// 2. Duplicate UID fails
	_, err = svc.CreateContentType(ctx, CreateTypeRequest{
		UID:  "article",
		Name: "Duplicate",
	})
	assert.ErrorIs(t, err, ErrContentTypeExists)

	// 3. Add Field
	err = svc.AddField(ctx, "article", ContentField{
		Name:      "Excerpt",
		FieldKey:  "excerpt",
		FieldType: fields.TypeLongText,
	})
	require.NoError(t, err)

	updatedCT, err := svc.GetContentType(ctx, "article")
	require.NoError(t, err)
	assert.Len(t, updatedCT.Fields, 4)
}

func TestService_DraftPublishAndVersionHistory(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()

	// Create Article type
	_, err := svc.CreateContentType(ctx, CreateTypeRequest{
		UID:  "article",
		Name: "Articles",
		Kind: KindCollection,
		Fields: []ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
			{Name: "Content", FieldKey: "content", FieldType: fields.TypeRichText},
		},
	})
	require.NoError(t, err)

	// Step 1: Create draft (Version 1)
	entry, err := svc.CreateEntry(ctx, "article", map[string]any{
		"title":   "Go CMS Architecture",
		"content": "Initial draft text",
	}, "user-1")
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, entry.Status)
	assert.Equal(t, 1, entry.DraftVersion)
	assert.Nil(t, entry.PublishedData)

	// Public access should fail because not yet published
	_, err = svc.GetEntry(ctx, "article", entry.ID, false)
	assert.ErrorIs(t, err, ErrNotPublished)

	// Preview access returns draft
	preview, err := svc.GetEntry(ctx, "article", entry.ID, true)
	require.NoError(t, err)
	assert.Equal(t, "Go CMS Architecture", preview.DraftData["title"])

	// Step 2: Update draft (Version 2)
	updated, err := svc.UpdateEntry(ctx, "article", entry.ID, map[string]any{
		"title":   "Go CMS Architecture v2",
		"content": "Updated content text",
	}, 1, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 2, updated.DraftVersion)
	assert.Equal(t, "Go CMS Architecture v2", updated.DraftData["title"])

	// Concurrency conflict test: Attempt update with stale version 1
	_, err = svc.UpdateEntry(ctx, "article", entry.ID, map[string]any{
		"title": "Stale update",
	}, 1, "user-1")
	assert.ErrorIs(t, err, ErrVersionMismatch)

	// Step 3: Publish (Published Version 2)
	published, err := svc.PublishEntry(ctx, "article", entry.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, StatusPublished, published.Status)
	assert.Equal(t, 2, published.PublishedVersion)
	assert.Equal(t, "Go CMS Architecture v2", published.PublishedData["title"])

	// Public access succeeds now!
	publicEntry, err := svc.GetEntry(ctx, "article", entry.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "Go CMS Architecture v2", publicEntry.PublishedData["title"])

	// Step 4: Modify draft while published (Draft Version 3)
	modified, err := svc.UpdateEntry(ctx, "article", entry.ID, map[string]any{
		"title":   "Go CMS Architecture (Draft in Progress)",
		"content": "Unpublished changes",
	}, 2, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 3, modified.DraftVersion)
	assert.Equal(t, 2, modified.PublishedVersion)
	assert.Equal(t, "Go CMS Architecture (Draft in Progress)", modified.DraftData["title"])
	// Public data is still untouched v2!
	assert.Equal(t, "Go CMS Architecture v2", modified.PublishedData["title"])

	// Step 5: Check Version History
	versions, err := svc.ListVersions(ctx, "article", entry.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 4) // create_draft (v1), update_draft (v2), publish (v2), update_draft (v3)

	// Step 6: Restore earlier version (Restore v1)
	restored, err := svc.RestoreVersion(ctx, "article", entry.ID, 1, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "Go CMS Architecture", restored.DraftData["title"])
	assert.Equal(t, 4, restored.DraftVersion) // Restore creates version 4
}

func TestService_SingleTypeSingletonEnforcement(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()

	// Create Single Type
	_, err := svc.CreateContentType(ctx, CreateTypeRequest{
		UID:  "homepage",
		Name: "Homepage Settings",
		Kind: KindSingle,
		Fields: []ContentField{
			{Name: "Hero Title", FieldKey: "hero_title", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
		},
	})
	require.NoError(t, err)

	// 1. Create first entry succeeds
	entry1, err := svc.CreateEntry(ctx, "homepage", map[string]any{
		"hero_title": "Welcome to our platform",
	}, "user-1")
	require.NoError(t, err)
	assert.NotEmpty(t, entry1.ID)

	// 2. Attempting second entry for single type fails
	_, err = svc.CreateEntry(ctx, "homepage", map[string]any{
		"hero_title": "Second homepage attempt",
	}, "user-1")
	assert.ErrorIs(t, err, ErrSingletonExists)
}

func TestService_ValidationAndFailureScenarios(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()

	// 1. Query non-existent content type
	_, err := svc.GetContentType(ctx, "ghost_type")
	assert.ErrorIs(t, err, ErrContentTypeNotFound)

	// 2. Create entry for non-existent content type
	_, err = svc.CreateEntry(ctx, "ghost_type", map[string]any{"title": "test"}, "user-1")
	assert.ErrorIs(t, err, ErrContentTypeNotFound)

	// 3. Create content type with required field
	_, err = svc.CreateContentType(ctx, CreateTypeRequest{
		UID:  "products",
		Name: "Products",
		Kind: KindCollection,
		Fields: []ContentField{
			{Name: "SKU", FieldKey: "sku", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
			{Name: "Price", FieldKey: "price", FieldType: fields.TypeNumber, Configuration: fields.ValidationRules{Required: true}},
		},
	})
	require.NoError(t, err)

	// 4. Create entry missing required field fails validation
	_, err = svc.CreateEntry(ctx, "products", map[string]any{
		"price": 100,
	}, "user-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	// 5. Delete non-existent entry returns ErrEntryNotFound
	err = svc.DeleteEntry(ctx, "products", "non-existent-id", false, "user-1")
	assert.ErrorIs(t, err, ErrEntryNotFound)
}

func TestService_AdvancedFieldConfigurationAndUIDGeneration(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()
	maxItems := 3
	_, err := svc.CreateContentType(ctx, CreateTypeRequest{
		UID: "shared.block", Name: "Block", Kind: KindComponent,
		Fields: []ContentField{{Name: "Heading", FieldKey: "heading", FieldType: fields.TypeShortText}},
	})
	require.NoError(t, err)
	svc.SetMediaLookup(func(_ context.Context, id string) (string, error) {
		if id == "media-1" {
			return "image/jpeg", nil
		}
		return "", ErrEntryNotFound
	})
	_, err = svc.CreateContentType(ctx, CreateTypeRequest{
		UID: "advanced", Name: "Advanced", Kind: KindCollection,
		Fields: []ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText},
			{Name: "Slug", FieldKey: "slug", FieldType: fields.TypeUID, Configuration: fields.ValidationRules{Unique: true, TargetField: "title"}},
			{Name: "Gallery", FieldKey: "gallery", FieldType: fields.TypeMedia, Configuration: fields.ValidationRules{Multiple: true, MaxItems: &maxItems}},
			{Name: "Blocks", FieldKey: "blocks", FieldType: fields.TypeRepeatableComponent, Configuration: fields.ValidationRules{Component: "shared.block", MaxItems: &maxItems}},
		},
	})
	require.NoError(t, err)

	entry, err := svc.CreateEntry(ctx, "advanced", map[string]any{
		"title":   "Hello Advanced Fields",
		"gallery": []any{"media-1"},
		"blocks":  []any{map[string]any{"heading": "Intro"}},
	}, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "hello-advanced-fields", entry.Slug)

	_, err = svc.CreateEntry(ctx, "advanced", map[string]any{
		"title":   "Hello Advanced Fields",
		"gallery": []any{},
		"blocks":  []any{},
	}, "user-2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unique")
}

func TestService_RelationTargetAndEntryValidation(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()
	_, err := svc.CreateContentType(ctx, CreateTypeRequest{UID: "authors", Name: "Authors", Kind: KindCollection, Fields: []ContentField{{Name: "Name", FieldKey: "name", FieldType: fields.TypeShortText}}})
	require.NoError(t, err)
	author, err := svc.CreateEntry(ctx, "authors", map[string]any{"name": "Jane Doe"}, "user-1")
	require.NoError(t, err)
	_, err = svc.CreateContentType(ctx, CreateTypeRequest{UID: "posts", Name: "Posts", Kind: KindCollection, Fields: []ContentField{{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText}, {Name: "Author", FieldKey: "author", FieldType: fields.TypeSingleRelation, Configuration: fields.ValidationRules{TargetType: "authors"}}, {Name: "Related", FieldKey: "related", FieldType: fields.TypeMultipleRelation, Configuration: fields.ValidationRules{TargetType: "posts"}}}})
	require.NoError(t, err)
	entry, err := svc.CreateEntry(ctx, "posts", map[string]any{"title": "Post", "author": author.ID, "related": []any{}}, "user-1")
	require.NoError(t, err)
	assert.Equal(t, author.ID, entry.DraftData["author"])
	_, err = svc.CreateEntry(ctx, "posts", map[string]any{"title": "Invalid", "author": "missing", "related": []any{}}, "user-1")
	assert.Error(t, err)
}

func TestService_EntryOwnershipAndSecretPersistence(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()
	for _, uid := range []string{"first", "second"} {
		_, err := svc.CreateContentType(ctx, CreateTypeRequest{UID: uid, Name: uid, Kind: KindCollection, Fields: []ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText},
			{Name: "Password", FieldKey: "password", FieldType: fields.TypeSecret},
		}})
		require.NoError(t, err)
	}
	entry, err := svc.CreateEntry(ctx, "first", map[string]any{"title": "One", "password": "VerySecret123!"}, "user-1")
	require.NoError(t, err)
	assert.NotEqual(t, "VerySecret123!", entry.DraftData["password"])
	assert.NotContains(t, entry.DraftData["password"], "VerySecret123!")

	_, err = svc.GetEntry(ctx, "second", entry.ID, true)
	assert.ErrorIs(t, err, ErrEntryNotFound)
	_, err = svc.UpdateEntry(ctx, "second", entry.ID, map[string]any{"title": "stolen"}, entry.DraftVersion, "user-2")
	assert.ErrorIs(t, err, ErrEntryNotFound)
	_, err = svc.PublishEntry(ctx, "second", entry.ID, "user-2")
	assert.ErrorIs(t, err, ErrEntryNotFound)
	assert.ErrorIs(t, svc.DeleteEntry(ctx, "second", entry.ID, false, "user-2"), ErrEntryNotFound)
}

func TestService_MediaAllowedTypesAcceptsHLSUnderVideo(t *testing.T) {
	svc, _ := setupTestContentService(t)
	ctx := context.Background()
	svc.SetMediaLookup(func(_ context.Context, id string) (string, error) {
		switch id {
		case "hls-1":
			return "application/vnd.apple.mpegurl", nil
		case "hls-2":
			return "application/x-mpegurl", nil
		case "png-1":
			return "image/png", nil
		case "pdf-1":
			return "application/pdf", nil
		default:
			return "", ErrEntryNotFound
		}
	})

	_, err := svc.CreateContentType(ctx, CreateTypeRequest{
		UID:  "videos",
		Name: "Videos",
		Kind: KindCollection,
		Fields: []ContentField{
			{
				Name:      "Clip",
				FieldKey:  "clip",
				FieldType: fields.TypeMedia,
				Configuration: fields.ValidationRules{
					Multiple:     true,
					AllowedTypes: []string{"image", "video"},
				},
			},
			{
				Name:      "Attachments",
				FieldKey:  "attachments",
				FieldType: fields.TypeMedia,
				Configuration: fields.ValidationRules{
					AllowedTypes: []string{"file"},
				},
			},
		},
	})
	require.NoError(t, err)

	entry, err := svc.CreateEntry(ctx, "videos", map[string]any{
		"clip":        []any{"hls-1", "hls-2", "png-1"},
		"attachments": "pdf-1",
	}, "user-1")
	require.NoError(t, err)
	assert.Equal(t, []any{"hls-1", "hls-2", "png-1"}, entry.DraftData["clip"])

	_, err = svc.CreateEntry(ctx, "videos", map[string]any{
		"clip":        []any{"pdf-1"},
		"attachments": "pdf-1",
	}, "user-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed MIME type")

	_, err = svc.CreateEntry(ctx, "videos", map[string]any{
		"clip":        []any{"png-1"},
		"attachments": "hls-1",
	}, "user-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed MIME type")
}
