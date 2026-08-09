package cmd

import (
	"fmt"
	"strings"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func youtubeActivityColumns() []outfmt.Column[*youtube.Activity] {
	return []outfmt.Column[*youtube.Activity]{
		{Header: "KIND", Value: func(activity *youtube.Activity) string { return activity.Kind }},
		{Header: "VIDEO_ID", Value: func(activity *youtube.Activity) string {
			if activity.ContentDetails == nil || activity.ContentDetails.Upload == nil {
				return ""
			}
			return sanitizeTab(activity.ContentDetails.Upload.VideoId)
		}},
		{Header: "TITLE", Value: func(activity *youtube.Activity) string {
			if activity.Snippet == nil {
				return ""
			}
			return sanitizeTab(activity.Snippet.Title)
		}},
		{Header: "PUBLISHED_AT", Value: func(activity *youtube.Activity) string {
			if activity.Snippet == nil {
				return ""
			}
			return sanitizeTab(activity.Snippet.PublishedAt)
		}},
	}
}

func youtubeVideoColumns() []outfmt.Column[*youtube.Video] {
	return youtubeResourceColumns(youtubeResourcePresentation[*youtube.Video]{
		id: func(video *youtube.Video) string { return video.Id },
		title: func(video *youtube.Video) string {
			return valueFrom(video.Snippet, func(snippet *youtube.VideoSnippet) string { return snippet.Title })
		},
		channel: func(video *youtube.Video) string {
			return valueFrom(video.Snippet, func(snippet *youtube.VideoSnippet) string { return snippet.ChannelTitle })
		},
		metricName: "VIEWS",
		metric: func(video *youtube.Video) string {
			return valueFrom(video.Statistics, func(statistics *youtube.VideoStatistics) string {
				return fmt.Sprintf("%d", statistics.ViewCount)
			})
		},
		published: func(video *youtube.Video) string {
			return valueFrom(video.Snippet, func(snippet *youtube.VideoSnippet) string { return snippet.PublishedAt })
		},
	})
}

func youtubePlaylistColumns() []outfmt.Column[*youtube.Playlist] {
	return youtubeResourceColumns(youtubeResourcePresentation[*youtube.Playlist]{
		id: func(playlist *youtube.Playlist) string { return playlist.Id },
		title: func(playlist *youtube.Playlist) string {
			return valueFrom(playlist.Snippet, func(snippet *youtube.PlaylistSnippet) string { return snippet.Title })
		},
		channel: func(playlist *youtube.Playlist) string {
			return valueFrom(playlist.Snippet, func(snippet *youtube.PlaylistSnippet) string { return snippet.ChannelTitle })
		},
		metricName: "VIDEO_COUNT",
		metric: func(playlist *youtube.Playlist) string {
			if playlist.ContentDetails == nil {
				return "0"
			}
			return fmt.Sprintf("%d", playlist.ContentDetails.ItemCount)
		},
		published: func(playlist *youtube.Playlist) string {
			return valueFrom(playlist.Snippet, func(snippet *youtube.PlaylistSnippet) string { return snippet.PublishedAt })
		},
	})
}

type youtubeResourcePresentation[T any] struct {
	id         func(T) string
	title      func(T) string
	channel    func(T) string
	metricName string
	metric     func(T) string
	published  func(T) string
}

func youtubeResourceColumns[T any](presentation youtubeResourcePresentation[T]) []outfmt.Column[T] {
	return []outfmt.Column[T]{
		{Header: "ID", Value: presentation.id},
		sanitizedColumn("TITLE", presentation.title),
		sanitizedColumn("CHANNEL", presentation.channel),
		{Header: presentation.metricName, Value: presentation.metric},
		sanitizedColumn("PUBLISHED_AT", presentation.published),
	}
}

func valueFrom[T any](value *T, extract func(*T) string) string {
	if value == nil {
		return ""
	}
	return extract(value)
}

func youtubePlaylistItemColumns() []outfmt.Column[*youtube.PlaylistItem] {
	return []outfmt.Column[*youtube.PlaylistItem]{
		{Header: "VIDEO_ID", Value: func(item *youtube.PlaylistItem) string {
			if item.ContentDetails != nil && item.ContentDetails.VideoId != "" {
				return sanitizeTab(item.ContentDetails.VideoId)
			}
			if item.Snippet != nil && item.Snippet.ResourceId != nil {
				return sanitizeTab(item.Snippet.ResourceId.VideoId)
			}
			return ""
		}},
		{Header: "TITLE", Value: func(item *youtube.PlaylistItem) string {
			if item.Snippet == nil {
				return ""
			}
			return sanitizeTab(item.Snippet.Title)
		}},
		{Header: "CHANNEL", Value: func(item *youtube.PlaylistItem) string {
			if item.Snippet == nil {
				return ""
			}
			return sanitizeTab(item.Snippet.VideoOwnerChannelTitle)
		}},
		{Header: "POSITION", Value: func(item *youtube.PlaylistItem) string {
			if item.Snippet == nil {
				return ""
			}
			return fmt.Sprintf("%d", item.Snippet.Position)
		}},
		{Header: "ITEM_ID", Value: func(item *youtube.PlaylistItem) string { return item.Id }},
		{Header: "PUBLISHED_AT", Value: func(item *youtube.PlaylistItem) string {
			if item.ContentDetails == nil {
				return ""
			}
			return sanitizeTab(item.ContentDetails.VideoPublishedAt)
		}},
	}
}

func youtubeCommentColumns() []outfmt.Column[*youtube.CommentThread] {
	return []outfmt.Column[*youtube.CommentThread]{
		{Header: "ID", Value: func(thread *youtube.CommentThread) string { return thread.Id }},
		{Header: "AUTHOR", Value: func(thread *youtube.CommentThread) string {
			snippet := youtubeTopLevelCommentSnippet(thread)
			if snippet == nil {
				return ""
			}
			return sanitizeTab(snippet.AuthorDisplayName)
		}},
		{Header: "TEXT", Value: func(thread *youtube.CommentThread) string {
			snippet := youtubeTopLevelCommentSnippet(thread)
			if snippet == nil {
				return ""
			}
			return sanitizeTab(youtubeCommentText(snippet.TextDisplay))
		}},
		{Header: "LIKE_COUNT", Value: func(thread *youtube.CommentThread) string {
			snippet := youtubeTopLevelCommentSnippet(thread)
			if snippet == nil {
				return "0"
			}
			return fmt.Sprintf("%d", snippet.LikeCount)
		}},
		{Header: "PUBLISHED_AT", Value: func(thread *youtube.CommentThread) string {
			snippet := youtubeTopLevelCommentSnippet(thread)
			if snippet == nil {
				return ""
			}
			return sanitizeTab(snippet.PublishedAt)
		}},
	}
}

func youtubeChannelColumns() []outfmt.Column[*youtube.Channel] {
	return []outfmt.Column[*youtube.Channel]{
		{Header: "ID", Value: func(channel *youtube.Channel) string { return channel.Id }},
		{Header: "TITLE", Value: func(channel *youtube.Channel) string {
			if channel.Snippet == nil {
				return ""
			}
			return sanitizeTab(channel.Snippet.Title)
		}},
		{Header: "SUBS", Value: func(channel *youtube.Channel) string {
			if channel.Statistics == nil {
				return ""
			}
			return fmt.Sprintf("%d", channel.Statistics.SubscriberCount)
		}},
		{Header: "VIDEOS", Value: func(channel *youtube.Channel) string {
			if channel.Statistics == nil {
				return ""
			}
			return fmt.Sprintf("%d", channel.Statistics.VideoCount)
		}},
		{Header: "VIEWS", Value: func(channel *youtube.Channel) string {
			if channel.Statistics == nil {
				return ""
			}
			return fmt.Sprintf("%d", channel.Statistics.ViewCount)
		}},
		{Header: "PUBLISHED_AT", Value: func(channel *youtube.Channel) string {
			if channel.Snippet == nil {
				return ""
			}
			return sanitizeTab(channel.Snippet.PublishedAt)
		}},
	}
}

func youtubeSearchColumns() []outfmt.Column[*youtube.SearchResult] {
	return []outfmt.Column[*youtube.SearchResult]{
		{Header: "KIND", Value: youtubeSearchResultType},
		{Header: "ID", Value: youtubeSearchResultID},
		{Header: "TITLE", Value: func(result *youtube.SearchResult) string {
			if result.Snippet == nil {
				return ""
			}
			return sanitizeTab(result.Snippet.Title)
		}},
		{Header: "CHANNEL", Value: func(result *youtube.SearchResult) string {
			if result.Snippet == nil {
				return ""
			}
			return sanitizeTab(result.Snippet.ChannelTitle)
		}},
		{Header: "PUBLISHED_AT", Value: func(result *youtube.SearchResult) string {
			if result.Snippet == nil {
				return ""
			}
			return sanitizeTab(result.Snippet.PublishedAt)
		}},
	}
}

func compactYouTubeRows[T any](rows []*T) []*T {
	filtered := make([]*T, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func youtubeTopLevelCommentSnippet(thread *youtube.CommentThread) *youtube.CommentSnippet {
	if thread == nil || thread.Snippet == nil || thread.Snippet.TopLevelComment == nil {
		return nil
	}
	return thread.Snippet.TopLevelComment.Snippet
}

func youtubeCommentText(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
	if len(text) > 60 {
		return text[:57] + "..."
	}
	return text
}

func youtubeSearchResultID(result *youtube.SearchResult) string {
	if result == nil || result.Id == nil {
		return ""
	}
	switch {
	case result.Id.VideoId != "":
		return result.Id.VideoId
	case result.Id.ChannelId != "":
		return result.Id.ChannelId
	case result.Id.PlaylistId != "":
		return result.Id.PlaylistId
	default:
		return ""
	}
}
