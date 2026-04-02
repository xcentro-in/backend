-- Migration: Add Matching Engine columns
-- Adds po_number, net_amount, match_status to invoice table;
-- remaining_balance, po_status to purchase_orders;
-- sku, qty_received to line_items;
-- and a match_results audit table.

-- 1. Add matching-related columns to invoice table
ALTER TABLE xcentro.invoice
  ADD COLUMN IF NOT EXISTS po_number TEXT,
  ADD COLUMN IF NOT EXISTS net_amount DECIMAL(20,2),
  ADD COLUMN IF NOT EXISTS match_status TEXT NOT NULL DEFAULT 'PENDING_MATCH';

-- 2. Add PO tracking columns to purchase_orders table
ALTER TABLE xcentro.purchase_orders
  ADD COLUMN IF NOT EXISTS remaining_balance DECIMAL(20,2),
  ADD COLUMN IF NOT EXISTS po_status TEXT NOT NULL DEFAULT 'OPEN';

-- 3. Add SKU and qty tracking to line_items table
ALTER TABLE xcentro.line_items
  ADD COLUMN IF NOT EXISTS sku TEXT,
  ADD COLUMN IF NOT EXISTS qty_received DECIMAL(12,4) NOT NULL DEFAULT 0;

-- 4. Create matching results table (audit trail)
CREATE TABLE IF NOT EXISTS xcentro.match_results (
  id UUID PRIMARY KEY DEFAULT uuidv7(),
  invoice_id UUID NOT NULL REFERENCES xcentro.invoice(id) ON DELETE CASCADE,
  purchase_order_id UUID REFERENCES xcentro.purchase_orders(id) ON DELETE SET NULL,
  match_status TEXT NOT NULL,
  match_details JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. Indexes for matching queries
CREATE INDEX IF NOT EXISTS idx_invoice_po_number       ON xcentro.invoice(po_number);
CREATE INDEX IF NOT EXISTS idx_invoice_match_status    ON xcentro.invoice(match_status);
CREATE INDEX IF NOT EXISTS idx_po_po_number            ON xcentro.purchase_orders(po_number);
CREATE INDEX IF NOT EXISTS idx_po_po_status            ON xcentro.purchase_orders(po_status);
CREATE INDEX IF NOT EXISTS idx_match_results_inv       ON xcentro.match_results(invoice_id);
CREATE INDEX IF NOT EXISTS idx_line_items_sku          ON xcentro.line_items(sku);
