package models

import "testing"

func TestWalletLedgerEntryValidate(t *testing.T) {
	valid := WalletLedgerEntry{Type: WalletLedgerEntryTopup, AmountCredits: 100}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid ledger entry rejected: %v", err)
	}

	for _, entry := range []WalletLedgerEntry{
		{Type: WalletLedgerEntryTopup, AmountCredits: 0},
		{Type: WalletLedgerEntryTopup, AmountCredits: -100},
		{Type: WalletLedgerEntryInvoiceDebit, AmountCredits: 100},
		{Type: WalletLedgerEntryType("invalid"), AmountCredits: 100},
	} {
		if err := entry.Validate(); err == nil {
			t.Fatalf("invalid ledger entry accepted: %#v", entry)
		}
	}
}
