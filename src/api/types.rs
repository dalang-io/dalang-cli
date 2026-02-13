use serde::{Deserialize, Serialize};
use std::collections::HashMap;

// --- Balance ---
#[derive(Debug, Deserialize)]
pub struct BalanceResponse {
    pub success: bool,
    pub data: BalanceData,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct BalanceData {
    pub balance: i64,
    pub total_topup: i64,
    pub total_spent: i64,
    #[serde(default)]
    pub total_commission: i64,
}

// --- Transactions ---
#[derive(Debug, Deserialize)]
pub struct TransactionsResponse {
    pub success: bool,
    pub data: TransactionsData,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct TransactionsData {
    pub transactions: Vec<Transaction>,
    pub total: i64,
    pub page: i64,
    pub limit: i64,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct Transaction {
    pub id: i64,
    #[serde(rename = "type")]
    pub tx_type: String,
    pub amount: i64,
    pub balance_after: i64,
    pub description: String,
    #[serde(default)]
    pub reference: String,
    pub created_at: String,
}

// --- VPS ---
#[derive(Debug, Deserialize, Serialize, Clone)]
pub struct VPS {
    pub id: String,
    pub name: String,
    #[serde(default)]
    pub display_name: String,
    pub status: String,
    #[serde(default)]
    pub vcpu: i32,
    #[serde(default)]
    pub ram: i32,
    #[serde(default)]
    pub storage: i32,
    #[serde(default)]
    pub storage_type: String,
    #[serde(default)]
    pub bandwidth: i32,
    #[serde(default)]
    pub price: i32,
    #[serde(default, rename = "expired_at")]
    pub expires_at: String,
    #[serde(default)]
    pub region: String,
    #[serde(default)]
    pub node: String,
    #[serde(default, rename = "ipv4_public")]
    pub public_ip: String,
    #[serde(default, rename = "ipv4_local")]
    pub local_ip: String,
    #[serde(default)]
    pub domain: String,
    #[serde(default)]
    pub custom_domain_enabled: i32,
    #[serde(default)]
    pub custom_domain_price: i32,
}

#[derive(Debug, Deserialize)]
pub struct VPSListResponse {
    pub success: bool,
    pub data: Vec<VPS>,
}

// --- Containers ---
#[derive(Debug, Deserialize, Serialize)]
pub struct Container {
    pub id: String,
    pub name: String,
    #[serde(default)]
    pub container_type: String,
    pub status: String,
    #[serde(default)]
    pub plan: String,
    #[serde(default)]
    pub expires_at: String,
}

#[derive(Debug, Deserialize)]
pub struct ContainerListData {
    pub containers: Vec<Container>,
}

#[derive(Debug, Deserialize)]
pub struct ContainerListResponse {
    pub success: bool,
    pub data: ContainerListData,
}

// --- Deployments ---
#[derive(Debug, Deserialize, Serialize)]
pub struct Deployment {
    pub id: String,
    pub name: String,
    pub status: String,
    #[serde(default)]
    pub repo_url: String,
    #[serde(default)]
    pub branch: String,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub expires_at: String,
}

#[derive(Debug, Deserialize)]
pub struct DeploymentListResponse {
    pub success: bool,
    pub data: Vec<Deployment>,
}

// --- VPS Usage ---
#[derive(Debug, Deserialize)]
pub struct VPSUsageResponse {
    pub success: bool,
    pub data: VPSUsageData,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct VPSUsageData {
    #[serde(default)]
    pub cpu_seconds: f64,
    #[serde(default)]
    pub memory_used: i64,
    #[serde(default)]
    pub memory_total: i64,
    #[serde(default)]
    pub disk_used: i64,
    #[serde(default)]
    pub disk_total: i64,
}

// --- Topup ---
#[derive(Debug, Deserialize)]
pub struct TopupResponse {
    pub success: bool,
    pub data: TopupData,
    #[serde(default)]
    pub message: String,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct TopupData {
    pub invoice_url: String,
    pub invoice_id: String,
    pub amount: i64,
}

// --- CLI Auth ---
#[derive(Debug, Deserialize)]
pub struct CLIAuthInitResponse {
    pub success: bool,
    pub data: CLIAuthInitData,
}

#[derive(Debug, Deserialize)]
pub struct CLIAuthInitData {
    pub device_code: String,
    pub user_code: String,
    pub verification_url: String,
    pub expires_in: i64,
    pub interval: i64,
}

#[derive(Debug, Deserialize)]
pub struct CLIAuthPollResponse {
    pub success: bool,
    pub data: CLIAuthPollData,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub error: String,
}

#[derive(Debug, Deserialize)]
pub struct CLIAuthPollData {
    #[serde(default)]
    pub access_token: String,
    #[serde(default)]
    pub refresh_token: String,
    #[serde(default)]
    pub email: String,
    #[serde(default)]
    pub expires_in: i64,
}

// --- Me ---
#[derive(Debug, Deserialize)]
pub struct MeResponse {
    pub success: bool,
    pub data: MeData,
}

#[derive(Debug, Deserialize)]
pub struct MeData {
    #[serde(default)]
    pub id: i64,
    pub email: String,
    #[serde(default)]
    pub role: String,
}

// --- Version Info (for update) ---
#[derive(Debug, Deserialize)]
pub struct VersionInfo {
    pub version: String,
    #[serde(default)]
    pub build_date: String,
}

// --- Generic response helpers ---
#[derive(Debug, Deserialize)]
pub struct GenericResponse {
    pub success: bool,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub error: String,
}

#[derive(Debug, Deserialize)]
pub struct OrderResponse {
    pub success: bool,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub data: OrderData,
}

#[derive(Debug, Deserialize, Default)]
pub struct OrderData {
    #[serde(default)]
    pub order_id: String,
    #[serde(default)]
    pub price: i64,
    #[serde(default)]
    pub invoice_url: String,
    #[serde(default)]
    pub status: String,
}

#[derive(Debug, Deserialize)]
pub struct UpgradeResponse {
    pub success: bool,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub error: String,
    #[serde(default)]
    pub data: UpgradeData,
}

#[derive(Debug, Deserialize, Default)]
pub struct UpgradeData {
    #[serde(default)]
    pub invoice_url: String,
    #[serde(default)]
    pub price: i64,
}

#[derive(Debug, Deserialize)]
pub struct ExtendResponse {
    pub success: bool,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub error: String,
    #[serde(default)]
    pub data: ExtendData,
}

#[derive(Debug, Deserialize, Default)]
pub struct ExtendData {
    #[serde(default)]
    pub invoice_url: String,
    #[serde(default)]
    pub price: i64,
    #[serde(default)]
    pub new_expiry: String,
}

#[derive(Debug, Deserialize)]
pub struct ExecResponse {
    pub success: bool,
    pub data: ExecData,
    #[serde(default)]
    pub message: String,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct ExecData {
    #[serde(default)]
    pub output: String,
    #[serde(default)]
    pub exit_code: i32,
}

#[derive(Debug, Deserialize)]
pub struct DomainEnableResponse {
    pub success: bool,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub data: DomainEnableData,
}

#[derive(Debug, Deserialize, Default)]
pub struct DomainEnableData {
    #[serde(default)]
    pub invoice_url: String,
    #[serde(default)]
    pub amount: i64,
}

#[derive(Debug, Deserialize)]
pub struct DomainListResponse {
    pub success: bool,
    pub data: Vec<DomainEntry>,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct DomainEntry {
    pub id: i64,
    pub domain: String,
    pub status: String,
    #[serde(default)]
    pub created_at: String,
}

#[derive(Debug, Deserialize)]
pub struct DomainAddResponse {
    pub success: bool,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub data: DomainAddData,
}

#[derive(Debug, Deserialize, Default)]
pub struct DomainAddData {
    #[serde(default)]
    pub domain: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub dns_records: Vec<HashMap<String, String>>,
    #[serde(default)]
    pub instructions: String,
}

#[derive(Debug, Deserialize)]
pub struct DomainVerifyResponse {
    pub success: bool,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub data: DomainVerifyData,
}

#[derive(Debug, Deserialize, Default)]
pub struct DomainVerifyData {
    #[serde(default)]
    pub domain: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub ssl_status: String,
    #[serde(default)]
    pub dns_records: Vec<HashMap<String, String>>,
}
