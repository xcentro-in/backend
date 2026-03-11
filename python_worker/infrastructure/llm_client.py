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
1. **Discount Logic**: 
   - If a percentage is explicitly mentioned (e.g., "20%"), set `discount` to the numeric percentage value (e.g., 20) and `discount_type` to "PERCENTAGE".
   - If only a currency amount is shown as a discount without a percentage, set `discount` to that amount and `discount_type` to "FLAT".
2. **Line Items**: 
   - Concatenate descriptions that span multiple lines or rows into a single string.
   - Clean all numeric strings (remove symbols like '$', '₹', or ',') before converting to a number.
3. **General**:
   - For `invoice_date`, always use YYYY-MM-DD format.
   - For `doc_type`, look for keywords like "Invoice", "Bill", or "Purchase Order".

### Schema:
{
  "doc_type": "INVOICE" or "PURCHASE_ORDER",
  "invoice_number": "string or null",
  "invoice_date": "YYYY-MM-DD or null",
  "po_number": "string or null",
  "vendor_name": "string or null",
  "vendor_gstin": "string or null",
  "buyer_gstin": "string or null",
  "total_amount": number or null,
  "discount": number or null,
  "discount_type": "FLAT" or "PERCENTAGE" or null,
  "line_items": [
    {
      "description": "string",
      "quantity": number,
      "unit_price": number,
      "tax": number or null,
      "amount": number
    }
  ]
}

### Rules:
- Return ONLY the JSON object. No markdown wrappers (like ```json), no preamble, no explanation.
- If a field is not visible, use null.
- All amount/rate/quantity fields MUST be numeric.
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

            response = client.models.generate_content(
                model=self._model,
                contents=[uploaded_file, _EXTRACTION_PROMPT],
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
            line_items=line_items,
            doc_type=doc_type,
            raw=data,
        )


def _safe_float(value) -> float | None:
    if value is None:
        return None
    try:
        return float(value)
    except (ValueError, TypeError):
        return None
