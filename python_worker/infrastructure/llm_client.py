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
   - If the document is neither an invoice nor a purchase order (e.g., a receipt, a letter, a random image), set `doc_type` to "OTHER".
2. **Discount Logic**: 
   - If a percentage is explicitly mentioned (e.g., "20%"), set `discount` to the numeric percentage value (e.g., 20) and `discount_type` to "PERCENTAGE".
   - If only a currency amount is shown as a discount without a percentage, set `discount` to that amount and `discount_type` to "FLAT".
3. **Line Items**: 
   - Concatenate descriptions that span multiple lines or rows into a single string.
   - Clean all numeric strings (remove symbols like '$', '₹', or ',') before converting to a number.
4. **Tax Fields**:
   - Extract SGST, CGST, and IGST amounts if they appear on the document.
   - These are Indian GST tax components. If they are not present, set to null.
5. **Shipping**:
   - Extract shipping / freight charges if present on the document.
6. **GSTIN Extraction (CRITICAL)**:
   - GSTIN is always exactly 15 alphanumeric characters, e.g., "27AADCB2230M1ZV".
   - Do NOT include spaces, hyphens, or extra characters. Strip any whitespace.
   - Vendor GSTIN typically appears near the seller/supplier header.
   - Buyer GSTIN typically appears near the "Bill To" or "Ship To" section.
   - If a GSTIN-like pattern is found but is clearly malformed (wrong length), set confidence to 0.2 and include it as-is.
7. **Vendor Name Extraction (CRITICAL)**:
   - Extract the **legal entity name** of the vendor/supplier, NOT a branch name or location.
   - For example, if the header says "ABC Enterprises - Mumbai Branch", extract "ABC Enterprises" as the vendor_name.
   - Prefer the name adjacent to the vendor GSTIN or at the top of the invoice header.
   - If both a trade name and legal name are visible, prefer the **legal name** (the one registered with GST).
8. **General**:
   - For `invoice_date`, always use YYYY-MM-DD format.

### Schema:
{
  "doc_type": "INVOICE" or "PURCHASE_ORDER" or "OTHER",
  "invoice_number": "string or null",
  "invoice_date": "YYYY-MM-DD or null",
  "po_number": "string or null",
  "vendor_name": "string or null",
  "vendor_gstin": "string or null",
  "buyer_gstin": "string or null",
  "total_amount": number or null,
  "discount": number or null,
  "discount_type": "FLAT" or "PERCENTAGE" or null,
  "sgst": number or null,
  "cgst": number or null,
  "igst": number or null,
  "shipping": number or null,
  "line_items": [
    {
      "description": "string",
      "quantity": number,
      "unit_price": number,
      "tax": number or null,
      "amount": number
    }
  ],
  "confidence_scores": {
    "invoice_number": 0.0 to 1.0,
    "invoice_date": 0.0 to 1.0,
    "po_number": 0.0 to 1.0,
    "vendor_name": 0.0 to 1.0,
    "vendor_gstin": 0.0 to 1.0,
    "buyer_gstin": 0.0 to 1.0,
    "total_amount": 0.0 to 1.0,
    "discount": 0.0 to 1.0,
    "sgst": 0.0 to 1.0,
    "cgst": 0.0 to 1.0,
    "igst": 0.0 to 1.0,
    "shipping": 0.0 to 1.0,
    "doc_type": 0.0 to 1.0,
    "line_items": 0.0 to 1.0
  }
}

### Rules:
- Return ONLY the JSON object. No markdown wrappers (like ```json), no preamble, no explanation.
- If a field is not visible, use null and set its confidence to 0.0.
- All amount/rate/quantity fields MUST be numeric.
- For confidence_scores, rate how confident you are in each extracted field value from 0.0 (no confidence / not found) to 1.0 (fully certain).
- If doc_type is "OTHER", you may leave all other fields as null with 0.0 confidence.
"""

_TEMPLATE_GEN_PROMPT = """
You are an expert at creating `invoice2data` YAML templates. Given the OCR text of an invoice and the target extracted values, create a strict YAML template that `invoice2data` can use to extract these fields from future invoices of the exact same layout.

Your goal is to extract:
- invoice_number
- date (invoice date)
- po_number
- issuer (vendor name)
- gstin or vendor_gstin
- buyer_gstin
- amount (total amount)
- discount
- discount_type
- sgst, cgst, igst
- freight or shipping

Rules:
1. Identify static keywords in the OCR text to use as the `issuer` or `keywords` fields to match this template.
2. Write concise regex patterns matching the OCR text. Use `(?P<field_name>...)` syntax for capture groups.
3. Use the provided actual data values as a reference to find where they appear in the OCR text.
4. Return ONLY valid YAML. Do not include markdown formatting (like ```yaml). No explanations.
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

    def generate_template(self, raw_ocr: str, draft_data: dict) -> str:
        try:
            client = self._get_client()
            prompt = (
                f"{_TEMPLATE_GEN_PROMPT}\n\n"
                f"### TARGET DATA TO EXTRACT (DRAFT DATA) ###\n{json.dumps(draft_data, indent=2)}\n\n"
                f"### RAW OCR TEXT ###\n{raw_ocr}\n"
            )
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
        """Convert LLM JSON → domain ExtractionResult."""
        line_items: list[LineItem] = []
        for li in data.get("line_items", []):
            line_items.append(
                LineItem(
                    description=li.get("description"),
                    quantity=_safe_float(li.get("quantity")),
                    unit_price=_safe_float(li.get("unit_price")),
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
            po_number=data.get("po_number"),
            vendor_name=data.get("vendor_name"),
            vendor_gstin=data.get("vendor_gstin"),
            buyer_gstin=data.get("buyer_gstin"),
            total_amount=_safe_float(data.get("total_amount")),
            discount=_safe_float(data.get("discount")),
            discount_type=data.get("discount_type"),
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
