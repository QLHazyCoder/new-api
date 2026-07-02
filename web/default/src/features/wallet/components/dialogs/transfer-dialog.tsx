/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useEffect } from 'react'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  displayAmountToQuota,
  getCurrencyDisplay,
  getCurrencyLabel,
  quotaToDisplayAmount,
} from '@/lib/currency'
import { formatQuota } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog } from '@/components/dialog'

interface TransferDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (amount: number) => Promise<boolean>
  availableQuota: number
  transferring: boolean
}

export function TransferDialog({
  open,
  onOpenChange,
  onConfirm,
  availableQuota,
  transferring,
}: TransferDialogProps) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState('')

  const { config, meta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const minQuota = Math.ceil(config.quotaPerUnit)
  const minDisplayAmount = quotaToDisplayAmount(minQuota)
  const availableDisplayAmount = quotaToDisplayAmount(availableQuota)
  const inputAmount = Number(amount)
  const transferQuota = displayAmountToQuota(inputAmount)
  const canTransfer = availableQuota >= minQuota
  const hasValidAmount =
    amount.trim() !== '' &&
    Number.isFinite(inputAmount) &&
    transferQuota >= minQuota &&
    transferQuota <= availableQuota
  const step = meta.kind === 'tokens' ? 1 : 0.000001

  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setAmount(canTransfer ? String(minDisplayAmount) : '')
    }
  }, [canTransfer, minDisplayAmount, open])

  const handleConfirm = async () => {
    if (!canTransfer) {
      toast.error(t('Insufficient rewards to meet the minimum transfer amount'))
      return
    }
    if (!Number.isFinite(inputAmount) || transferQuota < minQuota) {
      toast.error(t('Transfer amount must be at least {{amount}}', {
        amount: formatQuota(minQuota),
      }))
      return
    }
    if (transferQuota > availableQuota) {
      toast.error(t('Transfer amount exceeds available rewards'))
      return
    }

    const success = await onConfirm(transferQuota)
    if (success) {
      onOpenChange(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Transfer Rewards')}
      description={t('Move affiliate rewards to your main balance')}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      titleClassName='text-xl font-semibold'
      footerClassName='grid grid-cols-2 gap-2 sm:flex'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={transferring}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={transferring || !canTransfer || !hasValidAmount}
          >
            {transferring && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Transfer')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-3 sm:space-y-6 sm:py-4'>
        <div className='space-y-2'>
          <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
            {t('Available Rewards')}
          </Label>
          <div className='text-2xl font-semibold'>
            {formatQuota(availableQuota)}
          </div>
        </div>

        <div className='space-y-3'>
          <Label
            htmlFor='transfer-amount'
            className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
          >
            {t('Transfer Amount')} ({currencyLabel})
          </Label>
          <Input
            id='transfer-amount'
            type='number'
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            min={minDisplayAmount}
            max={availableDisplayAmount}
            step={step}
            className='font-mono text-lg'
            disabled={!canTransfer || transferring}
          />
          <div className='text-muted-foreground space-y-1 text-xs'>
            <p>
              {t('Minimum:')} {formatQuota(minQuota)}
            </p>
            {!canTransfer && (
              <p className='text-destructive'>
                {t('Available rewards have not reached the minimum transfer amount')}
              </p>
            )}
          </div>
        </div>
      </div>
    </Dialog>
  )
}
