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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, test, vi } from 'vitest'

const testState = vi.hoisted(() => ({
  emailVerification: true,
  isSending: false,
  isActive: false,
  secondsLeft: 30,
  sendCode: vi.fn(),
}))

vi.mock('@/features/auth/hooks/use-email-verification', () => ({
  useEmailVerification: () => ({
    isSending: testState.isSending,
    secondsLeft: testState.secondsLeft,
    isActive: testState.isActive,
    sendCode: testState.sendCode,
  }),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      email_verification: testState.emailVerification,
      oauth_register_enabled: false,
      wechat_login: false,
      user_agreement_enabled: false,
      privacy_policy_enabled: false,
    },
    loading: false,
    error: null,
  }),
}))

vi.mock('@/features/auth/hooks/use-turnstile', () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: false,
    turnstileSiteKey: '',
    turnstileToken: '',
    setTurnstileToken: vi.fn(),
    validateTurnstile: () => true,
  }),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({
    redirectToLogin: vi.fn(),
    handleLoginSuccess: vi.fn(),
  }),
}))

vi.mock('@/features/auth/components/oauth-providers', () => ({
  OAuthProviders: () => null,
}))

vi.mock('@/features/auth/api', () => ({
  register: vi.fn(),
  wechatLoginByCode: vi.fn(),
}))

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  resources: {
    en: {
      translation: {
        'Verification email sent. Please check your spam folder':
          'Verification email sent. Please check your spam folder',
        'A verification code was sent to {{email}}. Check your inbox first. If you do not see it, be sure to check your spam, junk, or promotions folder.':
          'A verification code was sent to {{email}}. Check your inbox first. If you do not see it, be sure to check your spam, junk, or promotions folder.',
      },
    },
  },
})

const { SignUpForm } = await import('../sign-up-form')

function renderForm() {
  return render(
    <I18nextProvider i18n={i18n}>
      <SignUpForm />
    </I18nextProvider>
  )
}

function enterEmail(email: string) {
  const input = screen.getByPlaceholderText('name@example.com')
  fireEvent.change(input, { target: { value: email } })
  return input
}

beforeEach(() => {
  testState.emailVerification = true
  testState.isSending = false
  testState.isActive = false
  testState.secondsLeft = 30
  testState.sendCode.mockReset()
})

describe('registration email verification notice', () => {
  test('shows the spam-folder guidance after a successful send and keeps it visible during countdown', async () => {
    testState.sendCode.mockResolvedValue(true)
    const view = renderForm()
    const email = 'alice@example.com'

    enterEmail(email)
    fireEvent.click(screen.getByRole('button', { name: 'Send code' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(
      'Verification email sent. Please check your spam folder'
    )
    expect(alert).toHaveTextContent(email)
    expect(alert).toHaveTextContent('spam')
    expect(testState.sendCode).toHaveBeenCalledWith(email)

    testState.isActive = true
    testState.secondsLeft = 30
    view.rerender(
      <I18nextProvider i18n={i18n}>
        <SignUpForm />
      </I18nextProvider>
    )

    expect(screen.getByRole('alert')).toHaveTextContent(email)

    testState.isActive = false
    testState.secondsLeft = 0
    view.rerender(
      <I18nextProvider i18n={i18n}>
        <SignUpForm />
      </I18nextProvider>
    )

    expect(screen.getByRole('alert')).toHaveTextContent(email)
  })

  test('does not show the notice when sending fails', async () => {
    testState.sendCode.mockResolvedValue(false)
    renderForm()

    enterEmail('alice@example.com')
    fireEvent.click(screen.getByRole('button', { name: 'Send code' }))

    await waitFor(() => expect(testState.sendCode).toHaveBeenCalled())
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  test('updates the notice to the latest email after a successful resend', async () => {
    testState.sendCode.mockResolvedValue(true)
    renderForm()
    const emailInput = enterEmail('alice@example.com')

    fireEvent.click(screen.getByRole('button', { name: 'Send code' }))
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('alice@example.com')

    fireEvent.change(emailInput, { target: { value: 'bob@example.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send code' }))

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('bob@example.com')
    )
    expect(screen.getByRole('alert')).not.toHaveTextContent('alice@example.com')
    expect(testState.sendCode).toHaveBeenNthCalledWith(2, 'bob@example.com')
  })

  test('does not render the notice when email verification is disabled', () => {
    testState.emailVerification = false
    renderForm()

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(
      screen.queryByPlaceholderText('Verification code')
    ).not.toBeInTheDocument()
  })
})
