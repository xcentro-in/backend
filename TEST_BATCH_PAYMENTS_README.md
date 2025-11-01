# Batch Payment Upload Test Data

## Test CSV File: `test_batch_payments.csv`

This CSV file contains 20 rows designed to test various matching scenarios in the batch payment upload feature.

---

## Test Data Breakdown

### ✅ GOOD INPUTS (Expected: Auto-matched with high confidence ≥75%)

1. **Row 1: TechCorp IT Services**
   - Exact match with account last 4 digits
   - Should match: `TechCorp IT Services` (relation_id: 0199f7c7-1234-7001-a003-111111111111)
   - Account: HDFC Bank ...5678
   - Amount: ₹50,000 (vendor payment)

2. **Row 2: Office Supplies - GOS**
   - Exact match with account last 4 digits
   - Should match: `Office Supplies - GOS` (relation_id: 0199f7c7-1234-7002-a003-222222222222)
   - Account: ICICI Bank ...1234
   - Amount: ₹15,000 (vendor payment)

3. **Row 3: BuildRight - Construction**
   - Close fuzzy match (missing "Contractor" but has account last 4)
   - Should match: `BuildRight - Construction Contractor` (relation_id: 0199f7c7-1234-7003-a003-333333333333)
   - Account: SBI ...9876
   - Amount: ₹2,50,000 (vendor payment)

4. **Row 4: Rajesh Kumar**
   - Exact match for employee payroll
   - Should match: `Rajesh Kumar - Engineering` (relation_id: 0199f7c8-1234-8001-e003-111111111111)
   - Account: HDFC Bank ...3456
   - Amount: ₹35,000 (payroll)

5. **Row 5: Priya Sharma**
   - Exact match for employee payroll
   - Should match: `Priya Sharma - HR` (relation_id: 0199f7c8-1234-8002-e003-222222222222)
   - Account: ICICI Bank ...3210
   - Amount: ₹42,000 (payroll)

6. **Row 11: Chandu Chai wala**
   - Exact match with account last 4
   - Should match: `Chandu Chai wala` (relation_id: d3c602a2-129d-4608-99c9-7f9beffce91e)
   - Account: AU Small Finance Bank ...4421
   - Amount: ₹2,000 (vendor payment)

---

### ⚠️ AMBIGUOUS INPUTS (Expected: Multiple matches or medium confidence 50-75%)

7. **Row 6: TechCorp**
   - Shortened name but correct account last 4
   - Should match: `TechCorp IT Services` (medium-high confidence ~80%)
   - Amount: ₹25,000 (vendor payment)

8. **Row 7: Office Supplies**
   - Missing "- GOS" suffix but correct account
   - Should match: `Office Supplies - GOS` (medium-high confidence ~85%)
   - Amount: ₹8,000 (vendor payment)

9. **Row 8: BuildRight Construction**
   - Different word order, no account last 4
   - Should match: `BuildRight - Construction Contractor` (medium confidence ~75%)
   - Amount: ₹1,00,000 (vendor payment)

10. **Row 9: Rajesh**
    - Only first name, but has account last 4
    - Could match: `Rajesh Kumar - Engineering` (medium confidence ~60%)
    - Amount: ₹5,000 (vendor payment) - Note: different context than usual payroll

11. **Row 12: TechCorp IT**
    - Partial name match
    - Should match: `TechCorp IT Services` (medium confidence ~85%)
    - Amount: ₹12,000 (vendor payment)

12. **Row 13: GOS Office**
    - Words reversed
    - Should match: `Office Supplies - GOS` (medium confidence ~60-70%)
    - Amount: ₹6,000 (vendor payment)

13. **Row 14: Kumar Rajesh**
    - Name reversed
    - Should match: `Rajesh Kumar - Engineering` (medium confidence ~70%)
    - Amount: ₹10,000 (payroll)

14. **Row 15: Sharma Priya**
    - Name reversed
    - Should match: `Priya Sharma - HR` (medium confidence ~70%)
    - Amount: ₹15,000 (payroll)

15. **Row 17: BuildRight**
    - Very short name, no account
    - Should match: `BuildRight - Construction Contractor` (low-medium confidence ~60%)
    - Amount: ₹80,000 (vendor payment)

16. **Row 18: Chandu**
    - Only first name but correct account last 4
    - Should match: `Chandu Chai wala` (medium-high confidence ~75%)
    - Amount: ₹1,500 (vendor payment)

---

### ❌ BAD INPUTS (Expected: No match found)

17. **Row 10: Unknown Vendor XYZ**
    - Completely unknown vendor
    - No account information
    - Should result in: NOT FOUND
    - Amount: ₹50,000 (vendor payment)

18. **Row 11: Test Vendor**
    - Exists in DB but with different account_id (019a0220-227e-7a6f-8722-4881f37372e1)
    - Account last 4 doesn't match (CSV has 3456, DB has ...3456 for different vendor)
    - Could be: AMBIGUOUS or NOT FOUND depending on name match threshold
    - Amount: ₹75,000 (vendor payment)

19. **Row 12: Completely Wrong Name**
    - Gibberish name
    - No account information
    - Should result in: NOT FOUND
    - Amount: ₹30,000 (vendor payment)

20. **Row 16: Random Company ABC**
    - Non-existent vendor
    - No account information
    - Should result in: NOT FOUND
    - Amount: ₹1,00,000 (vendor payment)

---

## Expected Results Summary

- **Total Rows**: 20
- **Expected Auto-matched**: 6-8 rows (confidence ≥ 75%)
- **Expected Ambiguous**: 8-10 rows (needs user review)
- **Expected Not Found**: 3-4 rows (no matches)
- **Total Amount**: ₹11,31,500 (in CSV rupees) = 113,150,000 paise

---

## Testing the Upload

```bash
# Upload the test CSV
curl -X POST http://localhost:8888/v1/batch_payments/upload \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "X-Business-ID: YOUR_BUSINESS_ID" \
  -F "csv_file=@test_batch_payments.csv"
```

### What to Verify:

1. **Matched Items**:
   - Should have high confidence scores (≥ 0.75)
   - Should have correct `matched_relation_id`
   - Should show matched payee name and account details

2. **Ambiguous Items**:
   - Should have `possible_matches` array
   - Each match should show confidence score
   - Should allow user to select correct match

3. **Not Found Items**:
   - Should have clear reason for failure
   - Should allow user to skip or add new payee

4. **Amounts**:
   - All amounts should be converted to paise (multiply by 100)
   - GST and TDS amounts should be preserved
   - Total amount should match sum of all rows

5. **Batch Record**:
   - Should create batch record in `xcentro.batch_payments`
   - Should create 20 items in `xcentro.batch_payment_items`
   - Should track matched/ambiguous/not_found counts

---

## Processing the Batch

After reviewing the preview, use the batch_id to process:

```bash
curl -X POST http://localhost:8888/v1/batch_payments/{batch_id}/process \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "X-Business-ID: YOUR_BUSINESS_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "ambiguous_mappings": {
      "6": "0199f7c7-1234-7001-a003-111111111111",
      "7": "0199f7c7-1234-7002-a003-222222222222"
    },
    "skip_rows": [10, 12, 16]
  }'
```

This will:
1. Resolve ambiguous mappings (row 6 → TechCorp, row 7 → Office Supplies)
2. Skip rows that couldn't be matched (rows 10, 12, 16)
3. Create DRAFT vouchers for all matched items
4. Return success/failure count

---

## Notes

- The fuzzy matching uses Levenshtein distance algorithm with 75% threshold
- Account last 4 digits help improve match confidence
- Payment context is preserved but not used for matching
- All vouchers are created in DRAFT status
- Payment happens separately via existing voucher payment flow
