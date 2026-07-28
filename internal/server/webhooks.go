package server

// Outbound webhooks are split across:
//
//	webhooks_store.go    — endpoint CRUD, event constants
//	webhooks_worker.go   — delivery, retry, StartWebhookWorker
//	webhooks_handlers.go — HTTP create/update/delete/test
