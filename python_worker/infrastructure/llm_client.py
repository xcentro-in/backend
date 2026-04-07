"""Gemini LLM-based extractor implementing the LLMExtractor port."""

from __future__ import annotations

import json
import logging
import os

from google import genai

from domain.entities import ExtractionResult, LineItem
from domain.enums import DocumentType
from domain.exceptions import LLMExtractionError
from domain.ports import LLMExtractor

logger = logging.getLogger(__name__)

# _EXTRACTION_PROMPT = """You are an expert invoice and purchase order data extraction system.
# Analyze the attached document and extract the following fields in valid JSON:

# {
#   "doc_type": "INVOICE" or "PURCHASE_ORDER",
#   "invoice_number": "string or null",
#   "invoice_date": "YYYY-MM-DD or null",
#   "po_number": "string or null",
#   "vendor_name": "string or null",
#   "vendor_gstin": "string or null",
#   "buyer_gstin": "string or null",
#   "total_amount": number or null,
#   "discount": number or null,
#   "discount_type": "FLAT" or "PERCENTAGE" or null,
#   "line_items": [
#     {
#       "description": "string",
#       "quantity": number,
#       "unit_price": number,
#       "tax": number or null,
#       "amount": number
#     }
#   ]
# }

# Rules:
# - Return ONLY the JSON object, no markdown, no explanation.
# - If a field is not visible in the document, use null.
# - Amounts should be numbers (not strings).
# - GSTIN must be the raw 15-character code if visible.
# """

_EXTRACTION_PROMPT = """
You are an expert invoice and purchase order data extraction system.
Analyze the attached document and extract the following fields into a single, valid JSON object.

### Extraction Logic:
1. **Document Classification**:
   - If the document is an invoice or bill, set `doc_type` to "INVOICE".
   - If the document is a purchase order, set `doc_type` to "PURCHASE_ORDER".
   - If the document is neither (e.g., a receipt, a letter, a random image), set `doc_type` to "OTHER".

2. **Discount Logic**:
   - If a percentage is explicitly mentioned (e.g., "20%"), set `discount` to the numeric value (e.g., 20) and `discount_type` to "PERCENTAGE".
   - If only a currency amount is shown as a discount, set `discount` to that amount and `discount_type` to "FLAT".

3. **Line Items (CRITICAL)**:
    - Concatenate descriptions that span multiple rows into a single string.
    - Extract `sku` (e.g. HSN/SAC, Item Code, Part Number, SKU, Product Code) if visible \u2014 this is the primary field used for PO-Invoice matching. Look for columns labelled "HSN", "HSN/SAC", "SAC", "Item Code", "Part No.", "SKU".
    - `unit_price` must be the per-unit price before tax.
    - `tax_rate` is the GST/tax percentage for this specific line (e.g., 18 for 18%). Set to null if not shown per-line; the document-level `gst_rate` will be used as fallback (see rule 4).
    - `tax` is the absolute tax amount for this line item in currency units. Set to null if not shown.
    - `amount` is the line total BEFORE tax (quantity \u00d7 unit_price). Do NOT include tax in this amount. The tax is tracked separately in `tax`.
    - Clean ALL numeric strings globally: remove currency symbols ('$', '\u20b9', '\u20ac', ',') before converting to numbers.

4. **GST Rate and Tax Fields (CRITICAL)**:
    - `gst_rate`: If the document shows a single GST rate that applies to all or most line items (e.g., "GST @ 18%", "Tax Rate: 12%", "CGST (12.00%)"), extract it as a number (e.g., 18 for 18% total GST, or 12 for CGST 12% + SGST 12% = 24% total \u2014 extract the INDIVIDUAL rate, not the sum). Set to null if the document has varying rates per line or no rate shown.
    - `sgst`, `cgst`, `igst`: Extract header-level ABSOLUTE CURRENCY AMOUNTS if present. These often appear near the bottom of the document in a summary table. Common formats include:
      - "CGST (12.00%)  37.80" \u2192 extract `cgst` = 37.80  (extract the currency amount, NOT the percentage)
      - "SGST @ 9%: \u20b91,800.00" \u2192 extract `sgst` = 1800.00
      - "IGST  18.00" \u2192 extract `igst` = 18.00
    - Set to null if absent.
    - **GST type is mutually exclusive**:
      - If IGST is present (Inter-state transaction), set `sgst` and `cgst` to null \u2014 they do not co-exist.
      - If SGST/CGST are present (Intra-state transaction), set `igst` to null \u2014 they do not co-exist.
    - Never report both IGST and SGST/CGST as non-zero. Pick the one that appears on the document.

5. **Shipping**:
    - Extract shipping / freight charges if present. This field is optional \u2014 set to null if absent.

6. **GSTIN Extraction (CRITICAL)**:
    - GSTIN is always exactly 15 alphanumeric characters, e.g., "27AADCB2230M1ZV".
    - Strip all spaces, hyphens, or extra characters.
    - Vendor GSTIN appears near the seller/supplier header.
    - Buyer GSTIN appears near the "Bill To" / "Ship To" section.
    - If a GSTIN-like pattern is malformed (wrong length), include it as-is and set its confidence to 0.2.

7. **Vendor Name Extraction (CRITICAL)**:
    - Extract the **legal entity name** of the vendor/supplier, NOT a branch name or location.
    - Example: "ABC Enterprises - Mumbai Branch" \u2192 extract "ABC Enterprises".
    - Prefer the name adjacent to the vendor GSTIN or at the top of the invoice header.
    - If both trade name and legal name are visible, prefer the **legal name** (GST-registered name).
    - The vendor name may appear under headers like "Vendor Address", "Supplier Address", "Sold By", or "Bill From". Look below these headers for the business entity name.
    - For Purchase Orders: the vendor/supplier is the RECIPIENT of the PO (the party being ordered from), not the issuer. The issuer is the buyer.

8. **Dates**:
    - `invoice_date`: the document issue date. Always use YYYY-MM-DD format.
    - `invoice_due_date`: the payment due date / last date for payment. Use YYYY-MM-DD or null if absent.

9. **Document-type specific rules**:
    - For **PURCHASE_ORDER** documents: set `invoice_number` to null \u2014 purchase orders do not have invoice numbers. The relevant reference field is `po_number`. Look for labels like "Purchase Order#", "PO No.", "PO Number", "Order No." to extract `po_number`.
    - For **INVOICE** documents: extract `invoice_number` if visible. `po_number` may also be present if the invoice references a PO.
    - **PO Number is CRITICAL**: This field is used for automated PO-Invoice matching. Always look for it prominently on the document.

10. **Amount Breakdown (CRITICAL)**:
     The amounts on an invoice follow this arithmetic chain. Use it to self-check your extraction:
     - `subtotal` = sum of all line item base amounts (before any GST). Null if not shown explicitly.
     - `gross_amount` = subtotal + sgst + cgst + igst (total after all taxes). Null if not shown explicitly.
     - `total_amount` = the final **Amount Due / Grand Total** prominently printed on the document.
       This is typically gross_amount \u2212 discount + shipping. It is the amount the buyer must pay.
     - `payment_terms`: stated payment terms (e.g. "Net 30", "Due on receipt"). Null if absent.
     - **Self-check**: if you can compute subtotal from line items, verify it roughly matches the extracted subtotal.
       If gross_amount \u2248 subtotal + sgst + cgst + igst, your tax extraction is consistent.

### Schema:
{
  "doc_type": "INVOICE" or "PURCHASE_ORDER" or "OTHER",
  "invoice_number": "string or null",
  "invoice_date": "YYYY-MM-DD or null",
  "invoice_due_date": "YYYY-MM-DD or null",
  "po_number": "string or null",
  "vendor_name": "string or null",
  "vendor_gstin": "string or null",
  "buyer_gstin": "string or null",
  "total_amount": number or null,
  "subtotal": number or null,
  "gross_amount": number or null,
  "discount": number or null,
  "discount_type": "FLAT" or "PERCENTAGE" or null,
  "gst_rate": number or null,
  "sgst": number or null,
  "cgst": number or null,
  "igst": number or null,
  "shipping": number or null,
  "payment_terms": "string or null",
  "line_items": [
    {
      "sku": "string or null",
      "description": "string or null",
      "quantity": number or null,
      "unit_price": number or null,
      "tax_rate": number or null,
      "tax": number or null,
      "amount": number or null
    }
  ],
  "confidence_scores": {
    "invoice_number": 0.0 to 1.0,
    "invoice_date": 0.0 to 1.0,
    "invoice_due_date": 0.0 to 1.0,
    "po_number": 0.0 to 1.0,
    "vendor_name": 0.0 to 1.0,
    "vendor_gstin": 0.0 to 1.0,
    "buyer_gstin": 0.0 to 1.0,
    "total_amount": 0.0 to 1.0,
    "subtotal": 0.0 to 1.0,
    "gross_amount": 0.0 to 1.0,
    "discount": 0.0 to 1.0,
    "gst_rate": 0.0 to 1.0,
    "sgst": 0.0 to 1.0,
    "cgst": 0.0 to 1.0,
    "igst": 0.0 to 1.0,
    "shipping": 0.0 to 1.0,
    "payment_terms": 0.0 to 1.0,
    "doc_type": 0.0 to 1.0,
    "line_items": 0.0 to 1.0
  }
}

### Rules:
- Return ONLY the JSON object. No markdown wrappers (like ```json), no preamble, no explanation.
- All amount/rate/quantity fields MUST be numeric (not strings).
- For confidence_scores, rate 0.0 (extremely uncertain) to 1.0 (fully certain).
- **Confidence for absent/inapplicable fields**: If a field is intentionally absent or not applicable to this document type (e.g., `invoice_number` on a Purchase Order, `igst` when CGST/SGST are used, `shipping` when no freight charges exist, `invoice_due_date` when no due date is listed), set its value to null AND set its confidence to **1.0** \u2014 because you are certain the field does not apply or is not present.
- **Low confidence (< 0.8)**: Only use low confidence when the document text is blurry, ambiguous, partially occluded, or you genuinely cannot determine the correct value.
- If doc_type is "OTHER", all other fields may be null with 0.0 confidence.
"""

_TEMPLATE_GEN_PROMPT = """
You are an expert at creating `invoice2data` YAML templates. Given the OCR text of a document and the validated extracted values, create a strict YAML template that `invoice2data` can use to extract the same fields from future documents of the identical layout.

### Fields to extract (include only those visible in the OCR text):
- invoice_number
- date (invoice date, in a format matching the OCR)
- invoice_due_date (payment due date, if present)
- po_number
- issuer (vendor legal name)
- vendor_gstin
- buyer_gstin
- amount (total_amount \u2014 the grand total / amount due)
- subtotal
- gross_amount
- discount
- discount_type
- sgst, cgst, igst
- shipping (freight charges)
- payment_terms

### Minimal valid template structure (use this as your reference):
```
issuer: Vendor Legal Name
keywords:
  - "Unique Keyword 1"
  - "Unique Keyword 2"

fields:
  invoice_number:
    - 'Invoice\\s*No[.:]?\\s*(?P<invoice_number>[A-Z0-9/-]+)'
  date:
    - '(?P<date>\\d{2}[/-]\\d{2}[/-]\\d{4})'
  amount:
    - 'Grand\\s*Total\\s*[:\\s]+(?P<amount>[\\d,]+\\.?\\d*)'

lines:
  start: 'Item\\s+Description\\s+HSN'
  end: 'Sub\\s*Total|Total|Notes'
  line: '(?P<description>.+?)\\s+(?P<sku>\\d{6,8})\\s+(?P<quantity>[\\d.]+)\\s+(?P<unit_price>[\\d,.]+)\\s+(?P<amount>[\\d,.]+)'

options:
  currency: INR
  decimal_separator: "."
  thousands_separator: ","
  date_formats:
    - "%d/%m/%Y"
    - "%d-%m-%Y"
    - "%Y-%m-%d"
```

### Rules:
1. **keywords**: Choose 2\u20134 strings that are unique to this vendor's invoice layout (e.g., company name, CIN, a distinctive header). These are what invoice2data uses to select this template.
2. **Regex patterns**: Write patterns that match the literal OCR text. Use `(?P<field_name>...)` named capture groups. Always anchor to nearby static text (labels, keywords) for precision.
3. **Only include fields visible in the OCR**: If a field's value does not appear in the OCR text, omit its regex entirely \u2014 do not guess.
4. **options block is REQUIRED**: Always include `currency`, `decimal_separator`, and `date_formats`. Infer date format from the `invoice_date` value in the provided data. Include at least 3 common date format variants.
5. **Numeric cleanup**: If amounts include commas as thousands separators, set `thousands_separator: ","`. If the decimal separator is a period, set `decimal_separator: "."`.
6. **Line items**: Add a `lines` block if the OCR clearly shows a tabular line-item section with consistent column alignment. Include `start` (regex matching the table header row), `end` (regex matching the row after the last item), and `line` (regex to capture each row with named groups like `sku`, `description`, `quantity`, `unit_price`, `amount`). Omit the `lines` block only if line items are absent or the table layout is too inconsistent to parse reliably.
7. Return ONLY valid YAML. No markdown fences (no ```yaml), no explanations, no preamble.
"""


class GeminiLLMExtractor(LLMExtractor):
    """Sends documents to Google Gemini for extraction.

    Client initialization is deferred to the first extract() call
    so the worker can start even when GEMINI_API_KEY is not yet set.
    """

    def __init__(self, api_key: str, model: str = "gemini-2.5-flash") -> None:
        self._api_key = api_key
        self._model = model
        self._client: genai.Client | None = None

    def _get_client(self) -> genai.Client:
        if self._client is None:
            if not self._api_key:
                raise LLMExtractionError("GEMINI_API_KEY is not configured")
            self._client = genai.Client(api_key=self._api_key)
        return self._client

    def extract(self, file_path: str) -> ExtractionResult:
        try:
            client = self._get_client()
            uploaded_file = client.files.upload(file=file_path)
            logger.info("Uploaded %s to Gemini as %s", file_path, uploaded_file.name)

            token_info = client.models.count_tokens(
                model=self._model,
                contents=[uploaded_file, _EXTRACTION_PROMPT],
            )
            logger.info("Estimated tokens before call: %s", token_info.total_tokens)

            response = client.models.generate_content(
                model=self._model,
                contents=[uploaded_file, _EXTRACTION_PROMPT],
            )

            usage = getattr(response, "usage_metadata", None)
            if usage:
                logger.info(
                    "[TOKENS] prompt=%d response=%d total=%d",
                    usage.prompt_token_count,
                    usage.candidates_token_count,
                    usage.total_token_count,
                )

            logger.info("Gemini raw response: \n%s", response)
            logger.info("====== Gemini response text: \n%s\n======================================================", response.text)

            raw_text = response.text.strip()

            # Strip markdown fences if present
            if raw_text.startswith("```"):
                raw_text = raw_text.split("\n", 1)[1]
            if raw_text.endswith("```"):
                raw_text = raw_text.rsplit("```", 1)[0]
            raw_text = raw_text.strip()

            data = json.loads(raw_text)
            return self._map(data)

        except json.JSONDecodeError as exc:
            raise LLMExtractionError(f"Gemini returned non-JSON: {exc}") from exc
        except Exception as exc:
            raise LLMExtractionError(f"Gemini API error: {exc}") from exc

    def generate_template(self, raw_ocr: str, draft_data: dict, historical_examples: list[dict] | None = None) -> str:
        try:
            client = self._get_client()
            prompt = f"{_TEMPLATE_GEN_PROMPT}\n\n"
            prompt += f"### TARGET DATA TO EXTRACT (CURRENT DOCUMENT) ###\n{json.dumps(draft_data, indent=2)}\n\n"
            if historical_examples:
                prompt += (
                    f"### PREVIOUSLY VALIDATED EXTRACTIONS FROM THE SAME TEMPLATE "
                    f"({len(historical_examples)} example(s)) ###\n"
                    f"{json.dumps(historical_examples, indent=2)}\n\n"
                )
            prompt += f"### RAW OCR TEXT ###\n{raw_ocr}\n"
            response = client.models.generate_content(
                model=self._model,
                contents=[prompt],
            )
            raw_text = response.text.strip()
            # Strip markdown fences if present
            if raw_text.startswith("```"):
                raw_text = raw_text.split("\n", 1)[1]
            if raw_text.endswith("```"):
                raw_text = raw_text.rsplit("```", 1)[0]
            if raw_text.startswith("yaml\n"):
                raw_text = raw_text[5:]
            return raw_text.strip()
        except Exception as exc:
            raise LLMExtractionError(f"Gemini API error during template generation: {exc}") from exc

    # -- mapping helpers ------------------------------------------------------

    @staticmethod
    def _map(data: dict) -> ExtractionResult:
        """Convert LLM JSON \u2192 domain ExtractionResult."""
        line_items: list[LineItem] = []
        for li in data.get("line_items", []):
            line_items.append(
                LineItem(
                    sku=li.get("sku") or None,
                    description=li.get("description"),
                    quantity=_safe_float(li.get("quantity")),
                    unit_price=_safe_float(li.get("unit_price")),
                    tax_rate=_safe_float(li.get("tax_rate")),
                    tax=_safe_float(li.get("tax")),
                    amount=_safe_float(li.get("amount")),
                )
            )

        doc_type_str = data.get("doc_type", "UNKNOWN").upper()
        try:
            doc_type = DocumentType(doc_type_str)
        except ValueError:
            doc_type = DocumentType.UNKNOWN

        # Parse confidence scores from LLM response
        raw_confidence = data.get("confidence_scores", {})
        confidence_scores: dict[str, float] = {}
        for key, val in raw_confidence.items():
            score = _safe_float(val)
            if score is not None:
                confidence_scores[key] = score

        return ExtractionResult(
            invoice_number=data.get("invoice_number"),
            invoice_date=data.get("invoice_date"),
            invoice_due_date=data.get("invoice_due_date"),
            po_number=data.get("po_number"),
            vendor_name=data.get("vendor_name"),
            vendor_gstin=data.get("vendor_gstin"),
            buyer_gstin=data.get("buyer_gstin"),
            total_amount=_safe_float(data.get("total_amount")),
            subtotal=_safe_float(data.get("subtotal")),
            gross_amount=_safe_float(data.get("gross_amount")),
            discount=_safe_float(data.get("discount")),
            discount_type=data.get("discount_type"),
            payment_terms=data.get("payment_terms"),
            gst_rate=_safe_float(data.get("gst_rate")),
            sgst=_safe_float(data.get("sgst")),
            cgst=_safe_float(data.get("cgst")),
            igst=_safe_float(data.get("igst")),
            shipping=_safe_float(data.get("shipping")),
            line_items=line_items,
            doc_type=doc_type,
            raw=data,
            confidence_scores=confidence_scores,
        )


def _safe_float(value) -> float | None:
    if value is None:
        return None
    try:
        return float(value)
    except (ValueError, TypeError):
        return None
