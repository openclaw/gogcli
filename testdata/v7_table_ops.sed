# SEDMAT v7 — Table Cell Operations
# ===================================
# Run AFTER v7_test.sed. Tables are numbered in document order:
#   Table 1: 3x2 pipe table (Col A/Col B)
#   Table 2: 4x3 pipe table (Feature/Status/Notes)
#   Table 3: 3x4 explicit empty table
#   Table 4: 4x3 merge test table
#   Table 5: 3x3 cell ops table
#
# Usage: gog docs sed <docId> -a <acct> < testdata/v7_table_ops.sed

# ============================================================
# TABLE 3: Fill cells + append row + append column
# [EXPECT: Headers bold, data filled, extra row & column added]
# ============================================================
s/|3|[1,1]/**ID**/
s/|3|[1,2]/**Name**/
s/|3|[1,3]/**Role**/
s/|3|[1,4]/**Status**/
s/|3|[2,1]/001/
s/|3|[2,2]/Alice/
s/|3|[2,3]/Engineer/
s/|3|[2,4]/Active/
s/|3|[3,1]/002/
s/|3|[3,2]/Bob/
s/|3|[3,3]/Designer/
s/|3|[3,4]/On Leave/
# Append row and column (content filled in separate ops after)
s/|3|[+1,1]//
s/|3|[1,+1]//
# Fill the appended cells
s/|3|[4,1]/003/
s/|3|[1,5]/**Dept**/

# ============================================================
# TABLE 4: Merge cells (header row spans 3 columns)
# [EXPECT: Row 1 is single merged cell with "Merged Header"]
# ============================================================
s/|4|[1,1]/**Merged Header**/
s/|4|[2,1]/R2C1/
s/|4|[2,2]/R2C2/
s/|4|[2,3]/R2C3/
s/|4|[3,1]/R3C1/
s/|4|[3,2]/R3C2/
s/|4|[3,3]/R3C3/
s/|4|[4,1]/R4C1/
s/|4|[4,2]/R4C2/
s/|4|[4,3]/R4C3/
s/|4|[1,1:1,3]/merge/

# ============================================================
# TABLE 5: Cell ops + wildcard formatting
# [EXPECT: Bold headers, data filled, Alice & Bob bold via wildcard]
# ============================================================
s/|5|[1,1]/**Name**/
s/|5|[1,2]/**Score**/
s/|5|[1,3]/**Grade**/
s/|5|[2,1]/Alice/
s/|5|[2,2]/95/
s/|5|[2,3]/A+/
s/|5|[3,1]/Bob/
s/|5|[3,2]/87/
s/|5|[3,3]/B+/
s/Alice|Bob/**&**/
