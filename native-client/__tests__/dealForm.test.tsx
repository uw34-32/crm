import { fireEvent, render, screen } from '@testing-library/react-native';

jest.mock('../src/lib/members', () => ({ useMembers: () => ({ members: [], status: 'ready', retry: () => undefined }) }));
jest.mock('expo-router', () => ({ router: {} }));

import { DealForm } from '../src/components/DealForm';

const initial = { title: 'Big deal', currency: 'USD', value_minor: 125050, pipeline_id: 'p1' };

async function setup(onSubmit = jest.fn()) {
  await render(<DealForm workspaceId="w1" initial={initial} submitLabel="Save" busy={false} error="" onSubmit={onSubmit} onCancel={() => undefined} />);
  return onSubmit;
}

describe('deal form', () => {
  it('follows the currency\'s decimals for a value the person has not touched', async () => {
    await setup();
    expect(screen.getByLabelText('Value').props.value).toBe('1250.50');
    await fireEvent.changeText(screen.getByLabelText('Currency'), 'JPY');
    expect(screen.getByLabelText('Value').props.value).toBe('125050');
  });

  it('keeps what the person typed when the currency changes', async () => {
    await setup();
    await fireEvent.changeText(screen.getByLabelText('Value'), '900');
    await fireEvent.changeText(screen.getByLabelText('Currency'), 'JPY');
    expect(screen.getByLabelText('Value').props.value).toBe('900');
  });

  it('submits without a decimals error after an untouched value changes currency', async () => {
    const onSubmit = await setup();
    await fireEvent.changeText(screen.getByLabelText('Currency'), 'JPY');
    await fireEvent.press(screen.getByRole('button', { name: 'Save' }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/no decimals/)).toBeNull();
  });
});
