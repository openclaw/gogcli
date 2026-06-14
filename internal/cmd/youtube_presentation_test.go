package cmd

import (
	"strings"
	"testing"

	youtube "google.golang.org/api/youtube/v3"
)

func TestYouTubePresentationSchemas(t *testing.T) {
	t.Parallel()

	t.Run("activities", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*youtube.Activity{{
			Kind: "youtube#activity",
			ContentDetails: &youtube.ActivityContentDetails{
				Upload: &youtube.ActivityContentDetailsUpload{VideoId: "video1"},
			},
			Snippet: &youtube.ActivitySnippet{Title: "Upload\tOne", PublishedAt: "2026-06-12T12:00:00Z"},
		}}, youtubeActivityColumns())
		assertTableOutput(
			t,
			got,
			"KIND\tVIDEO_ID\tTITLE\tPUBLISHED_AT\n"+
				"youtube#activity\tvideo1\tUpload One\t2026-06-12T12:00:00Z\n",
		)
	})

	t.Run("videos", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*youtube.Video{{
			Id:         "video1",
			Snippet:    &youtube.VideoSnippet{Title: "Video", ChannelTitle: "Channel", PublishedAt: "2026-06-12T12:00:00Z"},
			Statistics: &youtube.VideoStatistics{ViewCount: 99},
		}}, youtubeVideoColumns())
		assertTableOutput(
			t,
			got,
			"ID\tTITLE\tCHANNEL\tVIEWS\tPUBLISHED_AT\n"+
				"video1\tVideo\tChannel\t99\t2026-06-12T12:00:00Z\n",
		)
	})

	t.Run("playlists", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*youtube.Playlist{{
			Id:             "playlist1",
			Snippet:        &youtube.PlaylistSnippet{Title: "Playlist", ChannelTitle: "Channel", PublishedAt: "2026-06-12T12:00:00Z"},
			ContentDetails: &youtube.PlaylistContentDetails{ItemCount: 7},
		}}, youtubePlaylistColumns())
		assertTableOutput(
			t,
			got,
			"ID\tTITLE\tCHANNEL\tVIDEO_COUNT\tPUBLISHED_AT\n"+
				"playlist1\tPlaylist\tChannel\t7\t2026-06-12T12:00:00Z\n",
		)
	})

	t.Run("playlist items", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*youtube.PlaylistItem{
			{
				Id: "item1",
				Snippet: &youtube.PlaylistItemSnippet{
					Title:                  "Video",
					ChannelTitle:           "Playlist Owner",
					VideoOwnerChannelTitle: "Uploader",
					Position:               3,
				},
				ContentDetails: &youtube.PlaylistItemContentDetails{
					VideoId:          "video1",
					VideoPublishedAt: "2026-06-12T12:00:00Z",
				},
			},
			{
				Id: "item2",
				Snippet: &youtube.PlaylistItemSnippet{
					Title:        "Deleted video",
					ChannelTitle: "Playlist Owner",
					Position:     4,
				},
			},
		}, youtubePlaylistItemColumns())
		assertTableOutput(
			t,
			got,
			"VIDEO_ID\tTITLE\tCHANNEL\tPOSITION\tITEM_ID\tPUBLISHED_AT\n"+
				"video1\tVideo\tUploader\t3\titem1\t2026-06-12T12:00:00Z\n"+
				"\tDeleted video\t\t4\titem2\t\n",
		)
	})

	t.Run("comments", func(t *testing.T) {
		t.Parallel()
		text := strings.Repeat("x", 61)
		got := renderPlainTable(t, []*youtube.CommentThread{{
			Id: "comment1",
			Snippet: &youtube.CommentThreadSnippet{
				TopLevelComment: &youtube.Comment{
					Snippet: &youtube.CommentSnippet{
						AuthorDisplayName: "Ada",
						TextDisplay:       text,
						LikeCount:         3,
						PublishedAt:       "2026-06-12T12:00:00Z",
					},
				},
			},
		}}, youtubeCommentColumns())
		assertTableOutput(
			t,
			got,
			"ID\tAUTHOR\tTEXT\tLIKE_COUNT\tPUBLISHED_AT\n"+
				"comment1\tAda\t"+strings.Repeat("x", 57)+"...\t3\t2026-06-12T12:00:00Z\n",
		)
	})

	t.Run("channels", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*youtube.Channel{{
			Id:         "channel1",
			Snippet:    &youtube.ChannelSnippet{Title: "Channel", PublishedAt: "2026-06-12T12:00:00Z"},
			Statistics: &youtube.ChannelStatistics{SubscriberCount: 10, VideoCount: 4, ViewCount: 100},
		}}, youtubeChannelColumns())
		assertTableOutput(
			t,
			got,
			"ID\tTITLE\tSUBS\tVIDEOS\tVIEWS\tPUBLISHED_AT\n"+
				"channel1\tChannel\t10\t4\t100\t2026-06-12T12:00:00Z\n",
		)
	})

	t.Run("search", func(t *testing.T) {
		t.Parallel()
		got := renderPlainTable(t, []*youtube.SearchResult{{
			Id:      &youtube.ResourceId{VideoId: "video1"},
			Snippet: &youtube.SearchResultSnippet{Title: "Result", ChannelTitle: "Channel", PublishedAt: "2026-06-12T12:00:00Z"},
		}}, youtubeSearchColumns())
		assertTableOutput(
			t,
			got,
			"KIND\tID\tTITLE\tCHANNEL\tPUBLISHED_AT\n"+
				"video\tvideo1\tResult\tChannel\t2026-06-12T12:00:00Z\n",
		)
	})
}

func TestCompactYouTubeRows(t *testing.T) {
	t.Parallel()

	video := &youtube.Video{Id: "video1"}
	rows := compactYouTubeRows([]*youtube.Video{nil, video, nil})
	if len(rows) != 1 || rows[0] != video {
		t.Fatalf("rows = %#v, want only video1", rows)
	}
}
