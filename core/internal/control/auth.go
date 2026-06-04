// auth.go centralizes the ARM token scope used by the control-plane client.
// The actual credential (DefaultAzureCredential) is shared with the data-plane
// path in core/internal/foundry/auth.go — there is one identity, two scopes.
package control

// ARMScope is the Entra OAuth scope required for Azure Resource Manager calls.
// Tokens issued for this scope are accepted by management.azure.com only.
const ARMScope = "https://management.azure.com/.default"
