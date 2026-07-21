package tools

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/logging"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/desek/outlook-local-mcp/internal/validate"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

const maxGraphAttachmentSize int64 = 150 * 1024 * 1024

// NewHandleAddAttachment creates a draft-only local file attachment handler.
// Own drafts select direct or resumable upload by size. Shared drafts require a
// verified draft reference and support only the launch-validated direct path.
// The optional codec signs renewed shared draft provenance. Side effects include
// reading one allowlisted local file and mutating the exact routed Graph draft.
func NewHandleAddAttachment(retryCfg graph.RetryConfig, timeout time.Duration, roots []string, httpClient *http.Client, codecs ...*resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		messageID, err := target.draftID(request.GetString("message_id", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		filePath, err := request.RequireString("file_path")
		if err != nil || filePath == "" {
			return mcp.NewToolResultError("missing required parameter: file_path"), nil
		}
		if err := validate.ValidateResourceID(messageID, "message_id"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if result := verifyRoutedIsDraft(ctx, target, retryCfg, timeout, messageID, logging.Logger(ctx)); result != nil {
			return result, nil
		}
		canonical, err := validate.AttachmentPath(filePath, roots)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		file, err := os.Open(canonical)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open attachment: %v", err)), nil
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("stat attachment: %v", err)), nil
		}
		if info.Size() > maxGraphAttachmentSize {
			return mcp.NewToolResultError("attachment exceeds Microsoft Graph 150 MiB limit"), nil
		}
		if target.isShared() && info.Size() >= attachmentChunkSize {
			return mcp.NewToolResultError("shared mailbox attachment upload currently supports allowlisted files smaller than 3 MiB; no upload was started"), nil
		}
		contentType := mime.TypeByExtension(filepath.Ext(info.Name()))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		mode := "direct"
		attachmentID := ""
		if info.Size() < attachmentChunkSize {
			content, readErr := io.ReadAll(file)
			if readErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("read attachment: %v", readErr)), nil
			}
			attachment := models.NewFileAttachment()
			attachment.SetName(valuePtr(info.Name()))
			attachment.SetContentType(valuePtr(contentType))
			attachment.SetContentBytes(content)
			if err := revalidateMailTarget(ctx, target); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var created models.Attachmentable
			directCtx, directCancel := graph.WithTimeout(ctx, timeout)
			created, postErr := target.root.Messages().ByMessageId(messageID).Attachments().Post(directCtx, attachment, nil)
			directCancel()
			if postErr != nil {
				return mcp.NewToolResultError(attachmentMutationError(target, postErr)), nil
			}
			attachmentID = graph.SafeStr(created.GetId())
			if attachmentID == "" {
				if target.isShared() {
					return mcp.NewToolResultText(sharedAttachmentPartial(ctx, target, referenceCodec(codecs), messageID, "", info.Name(), info.Size(), contentType, "Graph accepted the attachment but returned no attachment ID")), nil
				}
				return mcp.NewToolResultError("attachment upload completed without a verified attachment ID"), nil
			}
		} else {
			mode = "resumable"
			item := models.NewAttachmentItem()
			attachmentType := models.FILE_ATTACHMENTTYPE
			item.SetAttachmentType(&attachmentType)
			item.SetName(valuePtr(info.Name()))
			item.SetContentType(valuePtr(contentType))
			size := info.Size()
			item.SetSize(&size)
			body := users.NewItemMessagesItemAttachmentsCreateUploadSessionPostRequestBody()
			body.SetAttachmentItem(item)
			var session models.UploadSessionable
			sessionCtx, sessionCancel := graph.WithTimeout(ctx, timeout)
			sessionErr := graph.RetryGraphCall(ctx, retryCfg, func() error {
				var callErr error
				session, callErr = target.root.Messages().ByMessageId(messageID).Attachments().CreateUploadSession().Post(sessionCtx, body, nil)
				return callErr
			})
			sessionCancel()
			if sessionErr != nil || session.GetUploadUrl() == nil {
				return mcp.NewToolResultError("failed to create attachment upload session"), nil
			}
			uploadCtx, uploadCancel := graph.WithTimeout(ctx, timeout)
			location, uploadErr := uploadAttachmentRanges(uploadCtx, httpClient, *session.GetUploadUrl(), file, size, retryCfg)
			uploadCancel()
			if uploadErr != nil {
				return mcp.NewToolResultError(uploadErr.Error()), nil
			}
			attachmentID, uploadErr = attachmentIDFromLocation(location)
			if uploadErr != nil {
				return mcp.NewToolResultError(uploadErr.Error()), nil
			}
		}
		verifyCtx, verifyCancel := graph.WithTimeout(ctx, timeout)
		var verified models.Attachmentable
		verifyErr := graph.RetryGraphCall(ctx, retryCfg, func() error {
			var callErr error
			verified, callErr = target.root.Messages().ByMessageId(messageID).Attachments().ByAttachmentId(attachmentID).Get(verifyCtx, nil)
			return callErr
		})
		verifyCancel()
		if verifyErr != nil || verified == nil || graph.SafeStr(verified.GetId()) != attachmentID {
			if target.isShared() {
				return mcp.NewToolResultText(sharedAttachmentPartial(ctx, target, referenceCodec(codecs), messageID, attachmentID, info.Name(), info.Size(), contentType, "attachment verification did not confirm the completed Graph mutation")), nil
			}
			return mcp.NewToolResultError("attachment upload completed but the attachment could not be verified"), nil
		}
		verifiedName := graph.SafeStr(verified.GetName())
		verifiedType := graph.SafeStr(verified.GetContentType())
		verifiedSizeValue := verified.GetSize()
		if verifiedName == "" || verifiedType == "" || verifiedSizeValue == nil {
			if target.isShared() {
				return mcp.NewToolResultText(sharedAttachmentPartial(ctx, target, referenceCodec(codecs), messageID, attachmentID, info.Name(), info.Size(), contentType, "attachment verification omitted name, size, or content type")), nil
			}
			return mcp.NewToolResultError("attachment verification omitted name, size, or content type"), nil
		}
		verifiedSize := int64(*verifiedSizeValue)
		accountLine := AccountInfoLine(ctx)
		if accountLine != "" {
			accountLine = "\n" + accountLine
		}
		response := fmt.Sprintf(
			"Attached %s (%d bytes, %s) to draft.\nDraft ID: %s\nAttachment ID: %s\nUpload: %s%s",
			verifiedName, verifiedSize, verifiedType, messageID, attachmentID, mode, accountLine,
		)
		response, err = sharedAttachmentConfirmation(response, target, referenceCodec(codecs), messageID)
		if err != nil {
			return mcp.NewToolResultText(sharedAttachmentPartial(ctx, target, referenceCodec(codecs), messageID, attachmentID, verifiedName, verifiedSize, verifiedType, err.Error())), nil
		}
		return mcp.NewToolResultText(response), nil
	}
}

// attachmentIDFromLocation extracts and URL-decodes the verified attachment ID
// from Graph's final resumable-upload Location response header.
func attachmentIDFromLocation(location string) (string, error) {
	parsed, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("parse attachment completion location: %w", err)
	}
	id := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if index := strings.LastIndex(id, "/"); index >= 0 {
		id = id[index+1:]
	}
	if id == "" {
		id = parsed.Opaque
	}
	id, err = url.PathUnescape(id)
	if err != nil || id == "" {
		return "", fmt.Errorf("attachment upload completion omitted a verified attachment ID")
	}
	return id, nil
}

// valuePtr returns a pointer to value for Graph SDK setter methods.
func valuePtr[T any](value T) *T { return &value }
