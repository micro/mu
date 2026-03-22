package app

import (
	"strings"
	"testing"
)

func TestSearchBar(t *testing.T) {
	result := SearchBar("/search", "Search...", "test query")
	if !strings.Contains(result, `action="/search"`) {
		t.Error("expected action attribute")
	}
	if !strings.Contains(result, `value="test query"`) {
		t.Error("expected query value")
	}
	if !strings.Contains(result, `placeholder="Search..."`) {
		t.Error("expected placeholder")
	}
}

func TestSearchBar_EscapesQuery(t *testing.T) {
	result := SearchBar("/search", "Search...", `<script>alert("xss")</script>`)
	if strings.Contains(result, "<script>") {
		t.Error("query should be HTML-escaped")
	}
}

func TestActionLink(t *testing.T) {
	result := ActionLink("/new", "Create Post")
	if !strings.Contains(result, `href="/new"`) {
		t.Error("expected href")
	}
	if !strings.Contains(result, "Create Post") {
		t.Error("expected label")
	}
	if !strings.Contains(result, `class="btn"`) {
		t.Error("expected btn class")
	}
}

func TestActionLink_EscapesLabel(t *testing.T) {
	result := ActionLink("/new", "<b>bold</b>")
	if strings.Contains(result, "<b>") {
		t.Error("label should be HTML-escaped")
	}
}

func TestGrid(t *testing.T) {
	result := Grid("<div>item</div>")
	if !strings.Contains(result, `class="card-grid"`) {
		t.Error("expected card-grid class")
	}
	if !strings.Contains(result, "<div>item</div>") {
		t.Error("expected content")
	}
}

func TestList(t *testing.T) {
	result := List("<div>item</div>")
	if !strings.Contains(result, `class="card-list"`) {
		t.Error("expected card-list class")
	}
}

func TestRow(t *testing.T) {
	result := Row("<div>item</div>")
	if !strings.Contains(result, `class="card-row"`) {
		t.Error("expected card-row class")
	}
}

func TestEmpty(t *testing.T) {
	result := Empty("Nothing here")
	if !strings.Contains(result, `class="empty"`) {
		t.Error("expected empty class")
	}
	if !strings.Contains(result, "Nothing here") {
		t.Error("expected message")
	}
}

func TestEmpty_EscapesMessage(t *testing.T) {
	result := Empty("<script>alert(1)</script>")
	if strings.Contains(result, "<script>") {
		t.Error("message should be HTML-escaped")
	}
}

func TestCardDiv(t *testing.T) {
	result := CardDiv("content here")
	if !strings.Contains(result, `class="card"`) {
		t.Error("expected card class")
	}
	if !strings.Contains(result, "content here") {
		t.Error("expected content")
	}
}

func TestCardDivClass(t *testing.T) {
	result := CardDivClass("highlight", "content")
	if !strings.Contains(result, `class="card highlight"`) {
		t.Error("expected card with extra class")
	}
}

func TestTags_Empty(t *testing.T) {
	result := Tags(nil, "")
	if result != "" {
		t.Errorf("expected empty string for nil tags, got %q", result)
	}
}

func TestTags_WithBaseURL(t *testing.T) {
	result := Tags([]string{"go", "web"}, "/blog?tag=")
	if !strings.Contains(result, `href="/blog?tag=go"`) {
		t.Error("expected tag link for 'go'")
	}
	if !strings.Contains(result, `href="/blog?tag=web"`) {
		t.Error("expected tag link for 'web'")
	}
}

func TestTags_WithoutBaseURL(t *testing.T) {
	result := Tags([]string{"go"}, "")
	if !strings.Contains(result, `<span class="tag">go</span>`) {
		t.Error("expected span tag without link")
	}
	if strings.Contains(result, "<a") {
		t.Error("should not contain links when no baseURL")
	}
}

func TestTitle_WithHref(t *testing.T) {
	result := Title("My Post", "/blog/post/1")
	if !strings.Contains(result, `href="/blog/post/1"`) {
		t.Error("expected link")
	}
	if !strings.Contains(result, "My Post") {
		t.Error("expected title text")
	}
}

func TestTitle_WithoutHref(t *testing.T) {
	result := Title("My Post", "")
	if strings.Contains(result, "<a") {
		t.Error("should not contain link when no href")
	}
	if !strings.Contains(result, `<span class="card-title">My Post</span>`) {
		t.Error("expected span with title")
	}
}

func TestMeta(t *testing.T) {
	result := Meta("by alice · 2h ago")
	if !strings.Contains(result, `class="card-meta"`) {
		t.Error("expected card-meta class")
	}
	if !strings.Contains(result, "by alice") {
		t.Error("expected content")
	}
}

func TestDesc(t *testing.T) {
	result := Desc("A short description")
	if !strings.Contains(result, `class="card-desc"`) {
		t.Error("expected card-desc class")
	}
	if !strings.Contains(result, "A short description") {
		t.Error("expected description text")
	}
}

func TestPage_Full(t *testing.T) {
	result := Page(PageOpts{
		Search:  "/search",
		Query:   "test",
		Action:  "/new",
		Label:   "Create",
		Filters: `<div class="filter">All</div>`,
		Content: "<div>items</div>",
	})
	if !strings.Contains(result, `action="/search"`) {
		t.Error("expected search bar")
	}
	if !strings.Contains(result, "Create") {
		t.Error("expected action button")
	}
	if !strings.Contains(result, "filter") {
		t.Error("expected filters")
	}
	if !strings.Contains(result, "items") {
		t.Error("expected content")
	}
}

func TestPage_EmptyState(t *testing.T) {
	result := Page(PageOpts{
		Empty: "No items found",
	})
	if !strings.Contains(result, "No items found") {
		t.Error("expected empty state message")
	}
}

func TestPage_DefaultLabel(t *testing.T) {
	result := Page(PageOpts{
		Action: "/new",
	})
	if !strings.Contains(result, "+ New") {
		t.Error("expected default label '+ New'")
	}
}

func TestCategory_WithHref(t *testing.T) {
	result := Category("tech", "/blog?tag=tech")
	if !strings.Contains(result, `<a href="/blog?tag=tech"`) {
		t.Error("expected link")
	}
	if !strings.Contains(result, "tech") {
		t.Error("expected label")
	}
}

func TestCategory_WithoutHref(t *testing.T) {
	result := Category("tech", "")
	if !strings.Contains(result, `<span class="category">tech</span>`) {
		t.Error("expected span")
	}
}

func TestAuthorLink(t *testing.T) {
	result := AuthorLink("alice", "Alice Smith")
	if !strings.Contains(result, `/@alice`) {
		t.Error("expected profile link")
	}
	if !strings.Contains(result, "Alice Smith") {
		t.Error("expected display name")
	}
}

func TestItemMeta_MultiParts(t *testing.T) {
	result := ItemMeta("tech", "by alice", "2h ago")
	if !strings.Contains(result, "tech · by alice · 2h ago") {
		t.Errorf("expected parts joined by ' · ', got %q", result)
	}
}

func TestItemMeta_SkipsEmpty(t *testing.T) {
	result := ItemMeta("tech", "", "2h ago")
	if strings.Contains(result, " ·  · ") {
		t.Error("should skip empty parts")
	}
	if !strings.Contains(result, "tech · 2h ago") {
		t.Errorf("expected 'tech · 2h ago', got %q", result)
	}
}

func TestItemMeta_AllEmpty(t *testing.T) {
	result := ItemMeta("", "", "")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestDeleteButton(t *testing.T) {
	result := DeleteButton("/blog/post/1", "Remove", "Delete this?")
	if !strings.Contains(result, "Delete this?") {
		t.Error("expected custom confirm message")
	}
	if !strings.Contains(result, "Remove") {
		t.Error("expected custom label")
	}
}

func TestDeleteButton_Defaults(t *testing.T) {
	result := DeleteButton("/blog/post/1", "", "")
	if !strings.Contains(result, "Delete") {
		t.Error("expected default label 'Delete'")
	}
	if !strings.Contains(result, "Are you sure?") {
		t.Error("expected default confirm message")
	}
}

func TestReplyForm(t *testing.T) {
	result := ReplyForm("/comment", "Write something...", "thread_id", "123")
	if !strings.Contains(result, `action="/comment"`) {
		t.Error("expected form action")
	}
	if !strings.Contains(result, `name="thread_id"`) {
		t.Error("expected hidden parent field")
	}
	if !strings.Contains(result, `value="123"`) {
		t.Error("expected parent value")
	}
}

func TestReplyForm_NoParent(t *testing.T) {
	result := ReplyForm("/comment", "", "", "")
	if strings.Contains(result, `type="hidden"`) {
		t.Error("should not have hidden input without parent")
	}
	if !strings.Contains(result, "Write a reply...") {
		t.Error("expected default placeholder")
	}
}

func TestSection(t *testing.T) {
	result := Section("Comments")
	if !strings.Contains(result, "<h3") {
		t.Error("expected h3 tag")
	}
	if !strings.Contains(result, "Comments") {
		t.Error("expected title text")
	}
}

func TestLoginPrompt(t *testing.T) {
	result := LoginPrompt("post", "/blog")
	if !strings.Contains(result, `/login?redirect=/blog`) {
		t.Error("expected login redirect")
	}
	if !strings.Contains(result, "post") {
		t.Error("expected action text")
	}
}

func TestBackLink(t *testing.T) {
	result := BackLink("Back to Blog", "/blog")
	if !strings.Contains(result, `href="/blog"`) {
		t.Error("expected href")
	}
	if !strings.Contains(result, "← Back to Blog") {
		t.Error("expected back link text")
	}
}
