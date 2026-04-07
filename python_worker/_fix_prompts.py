"""One-shot script to update extraction prompts in llm_client.py"""

filepath = 'infrastructure/llm_client.py'

with open(filepath, 'r', encoding='utf-8') as f:
    content = f.read()

# 1. Fix Line Items rule - SKU mentions HSN/SAC, amount is pre-tax
old_line_items = (
    '3. **Line Items (CRITICAL)**:\n'
    '    - Concatenate descriptions that span multiple rows into a single string.\n'
    '    - Extract `sku` (item code / part number / HSN code / SKU) if visible \u2014 this is the primary field used for matching.\n'
    '    - `unit_price` must be the per-unit price before tax.\n'
    '    - `tax_rate` is the GST/tax percentage for this specific line (e.g., 18 for 18%). Set to null if not shown per-line; the document-level `gst_rate` will be used as fallback (see rule 4).\n'
    '    - `tax` is the absolute tax amount for this line item in currency units. Set to null if not shown.\n'
    '    - `amount` is the final line total including tax (quantity \u00d7 unit_price + tax).\n'
    '    - Clean ALL numeric strings globally: remove currency symbols (\'$\', \'\u20b9\', \'\u20ac\', \',\') before converting to numbers.'
)

new_line_items = (
    '3. **Line Items (CRITICAL)**:\n'
    '    - Concatenate descriptions that span multiple rows into a single string.\n'
    '    - Extract `sku` (e.g. HSN/SAC, Item Code, Part Number, SKU, Product Code) if visible \u2014 this is the primary field used for PO-Invoice matching. Look for columns labelled "HSN", "HSN/SAC", "SAC", "Item Code", "Part No.", "SKU".\n'
    '    - `unit_price` must be the per-unit price before tax.\n'
    '    - `tax_rate` is the GST/tax percentage for this specific line (e.g., 18 for 18%). Set to null if not shown per-line; the document-level `gst_rate` will be used as fallback (see rule 4).\n'
    '    - `tax` is the absolute tax amount for this line item in currency units. Set to null if not shown.\n'
    '    - `amount` is the line total BEFORE tax (quantity \u00d7 unit_price). Do NOT include tax in this amount. The tax is tracked separately in `tax`.\n'
    '    - Clean ALL numeric strings globally: remove currency symbols (\'$\', \'\u20b9\', \'\u20ac\', \',\') before converting to numbers.'
)

assert old_line_items in content, "Line items block not found!"
content = content.replace(old_line_items, new_line_items)
print("[1/4] Line items rule updated")

# 2. Fix GST Rate rule
old_gst = (
    '4. **GST Rate and Tax Fields (CRITICAL)**:\n'
    '    - `gst_rate`: If the document shows a single GST rate that applies to all or most line items (e.g., "GST @ 18%", "Tax Rate: 12%"), extract it as a number (e.g., 18). Set to null if the document has varying rates per line or no rate shown.\n'
    '    - `sgst`, `cgst`, `igst`: Extract header-level amounts in currency units if present. Set to null if absent.\n'
    '    - **GST type is mutually exclusive**:\n'
    '      - If IGST is present (Inter-state transaction), set `sgst` and `cgst` to null \u2014 they do not co-exist.\n'
    '      - If SGST/CGST are present (Intra-state transaction), set `igst` to null \u2014 they do not co-exist.\n'
    '    - Never report both IGST and SGST/CGST as non-zero. Pick the one that appears on the document.'
)

new_gst = (
    '4. **GST Rate and Tax Fields (CRITICAL)**:\n'
    '    - `gst_rate`: If the document shows a single GST rate that applies to all or most line items (e.g., "GST @ 18%", "Tax Rate: 12%", "CGST (12.00%)"), extract it as a number (e.g., 18 for 18% total GST, or 12 for CGST 12% + SGST 12% = 24% total \u2014 extract the INDIVIDUAL rate, not the sum). Set to null if the document has varying rates per line or no rate shown.\n'
    '    - `sgst`, `cgst`, `igst`: Extract header-level ABSOLUTE CURRENCY AMOUNTS if present. These often appear near the bottom of the document in a summary table. Common formats include:\n'
    '      - "CGST (12.00%)  37.80" \u2192 extract `cgst` = 37.80  (extract the currency amount, NOT the percentage)\n'
    '      - "SGST @ 9%: \u20b91,800.00" \u2192 extract `sgst` = 1800.00\n'
    '      - "IGST  18.00" \u2192 extract `igst` = 18.00\n'
    '    - Set to null if absent.\n'
    '    - **GST type is mutually exclusive**:\n'
    '      - If IGST is present (Inter-state transaction), set `sgst` and `cgst` to null \u2014 they do not co-exist.\n'
    '      - If SGST/CGST are present (Intra-state transaction), set `igst` to null \u2014 they do not co-exist.\n'
    '    - Never report both IGST and SGST/CGST as non-zero. Pick the one that appears on the document.'
)

assert old_gst in content, "GST block not found!"
content = content.replace(old_gst, new_gst)
print("[2/4] GST rule updated")

# 3. Fix Vendor Name rule
old_vendor = (
    '7. **Vendor Name Extraction (CRITICAL)**:\n'
    '    - Extract the **legal entity name** of the vendor/supplier, NOT a branch name or location.\n'
    '    - Example: "ABC Enterprises - Mumbai Branch" \u2192 extract "ABC Enterprises".\n'
    '    - Prefer the name adjacent to the vendor GSTIN or at the top of the invoice header.\n'
    '    - If both trade name and legal name are visible, prefer the **legal name** (GST-registered name).'
)

new_vendor = (
    '7. **Vendor Name Extraction (CRITICAL)**:\n'
    '    - Extract the **legal entity name** of the vendor/supplier, NOT a branch name or location.\n'
    '    - Example: "ABC Enterprises - Mumbai Branch" \u2192 extract "ABC Enterprises".\n'
    '    - Prefer the name adjacent to the vendor GSTIN or at the top of the invoice header.\n'
    '    - If both trade name and legal name are visible, prefer the **legal name** (GST-registered name).\n'
    '    - The vendor name may appear under headers like "Vendor Address", "Supplier Address", "Sold By", or "Bill From". Look below these headers for the business entity name.\n'
    '    - For Purchase Orders: the vendor/supplier is the RECIPIENT of the PO (the party being ordered from), not the issuer. The issuer is the buyer.'
)

assert old_vendor in content, "Vendor block not found!"
content = content.replace(old_vendor, new_vendor)
print("[3/4] Vendor name rule updated")

# 4. Fix Document-type specific rules
old_doctype = (
    '9. **Document-type specific rules**:\n'
    '    - For **PURCHASE_ORDER** documents: set `invoice_number` to null \u2014 purchase orders do not have invoice numbers. The relevant reference field is `po_number`.\n'
    '    - For **INVOICE** documents: extract `invoice_number` if visible. `po_number` may also be present if the invoice references a PO.'
)

new_doctype = (
    '9. **Document-type specific rules**:\n'
    '    - For **PURCHASE_ORDER** documents: set `invoice_number` to null \u2014 purchase orders do not have invoice numbers. The relevant reference field is `po_number`. Look for labels like "Purchase Order#", "PO No.", "PO Number", "Order No." to extract `po_number`.\n'
    '    - For **INVOICE** documents: extract `invoice_number` if visible. `po_number` may also be present if the invoice references a PO.\n'
    '    - **PO Number is CRITICAL**: This field is used for automated PO-Invoice matching. Always look for it prominently on the document.'
)

assert old_doctype in content, "Doctype block not found!"
content = content.replace(old_doctype, new_doctype)
print("[4/4] Document-type rules updated")

with open(filepath, 'w', encoding='utf-8') as f:
    f.write(content)

print("\nAll 4 prompt replacements applied successfully!")
