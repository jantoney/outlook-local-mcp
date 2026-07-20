package auth

import "github.com/Azure/azure-sdk-for-go/sdk/azidentity"

// TokenTenantContextFromAuthState reads the validated tenant evidence already
// persisted by the configured authentication implementation. Azure Identity
// records contribute AuthenticationRecord.TenantID; the manual MSAL flow
// contributes the authenticated account Realm. Missing or invalid state is
// classified as unknown. The function performs only local file reads.
func TokenTenantContextFromAuthState(authMethod, authRecordPath string) TokenTenantContext {
	if authRecordPath == "" {
		return TokenTenantUnknown
	}
	if authMethod == "auth_code" {
		account, found, err := loadAuthCodeAccount(authRecordPath)
		if err != nil || !found {
			return TokenTenantUnknown
		}
		return ClassifyTokenTenantContext(account.Realm)
	}
	return ClassifyTokenTenantContext(LoadAuthRecord(authRecordPath).TenantID)
}

// TokenTenantContextFromRecord classifies the validated tenant ID carried by
// an Azure Identity authentication record. It exists as a narrow seam for
// authentication flows that have the successful record in memory.
func TokenTenantContextFromRecord(record azidentity.AuthenticationRecord) TokenTenantContext {
	return ClassifyTokenTenantContext(record.TenantID)
}
