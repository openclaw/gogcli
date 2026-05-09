package cmd

const (
	driveFileListFields = "nextPageToken, files(id, name, mimeType, size, modifiedTime, parents, webViewLink, owners(emailAddress), driveId, hasThumbnail, thumbnailLink)"
	driveFileGetFields  = "id, name, mimeType, size, modifiedTime, createdTime, parents, webViewLink, description, starred, driveId, hasThumbnail, thumbnailLink"
)
