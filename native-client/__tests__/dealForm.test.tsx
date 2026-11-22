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

describe('the expected close date', () => {
  const today = () => {
    const d = new Date();
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  };

  it('starts as today on a new deal, and the status as won', async () => {
    await render(<DealForm workspaceId="w1" creating pipelines={[{ id: 'p1', name: 'Sales' }]} submitLabel="Save" busy={false} error="" onSubmit={() => undefined} onCancel={() => undefined} />);
    expect(screen.getByLabelText('Expected close date').props.value).toBe(today());
    expect(screen.getByRole('radio', { name: 'Won', checked: true })).toBeTruthy();
  });

  it("keeps an edited deal's own date", async () => {
    await render(<DealForm workspaceId="w1" initial={{ ...initial, expected_close_date: '2027-03-14' }} submitLabel="Save" busy={false} error="" onSubmit={() => undefined} onCancel={() => undefined} />);
    expect(screen.getByLabelText('Expected close date').props.value).toBe('2027-03-14');
  });

  it('stays blank on an edited deal that had no date', async () => {
    await render(<DealForm workspaceId="w1" initial={initial} submitLabel="Save" busy={false} error="" onSubmit={() => undefined} onCancel={() => undefined} />);
    expect(screen.getByLabelText('Expected close date').props.value).toBe('');
  });
});

describe('the extra fields', () => {
  const creating = () => render(<DealForm workspaceId="w1" creating pipelines={[{ id: 'p1', name: 'Sales' }, { id: 'p2', name: 'Other' }]} submitLabel="Record sale" busy={false} error="" onSubmit={() => undefined} onCancel={() => undefined} />);

  it('stay behind "More details" on a new deal, so recording a sale fits one screen', async () => {
    await creating();
    expect(screen.queryByText('Owner')).toBeNull();
    expect(screen.queryByText('Pipeline')).toBeNull();
    expect(screen.getByRole('button', { name: 'Record sale' })).toBeTruthy();
    await fireEvent.press(screen.getByRole('button', { name: 'More details' }));
    expect(screen.getByText('Pipeline')).toBeTruthy();
    expect(screen.getByText('Owner')).toBeTruthy();
    await fireEvent.press(screen.getByRole('button', { name: 'Fewer details' }));
    expect(screen.queryByText('Owner')).toBeNull();
  });

  it('are all shown when editing a deal', async () => {
    await setup();
    expect(screen.getByText('Owner')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'More details' })).toBeNull();
  });

  it('still go with the deal when they were never opened', async () => {
    const onSubmit = jest.fn();
    await render(<DealForm workspaceId="w1" creating pipelines={[{ id: 'p1', name: 'Sales' }]} submitLabel="Record sale" busy={false} error="" onSubmit={onSubmit} onCancel={() => undefined} />);
    await fireEvent.changeText(screen.getByLabelText('Title'), 'Dress');
    await fireEvent.press(screen.getByRole('button', { name: 'Record sale' }));
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ title: 'Dress', pipeline_id: 'p1', status: 'won' }));
  });
});

describe('the date follows the status', () => {
  const dateInput = () => screen.getByLabelText('Expected close date');
  const creating = (onSubmit = jest.fn()) => render(<DealForm workspaceId="w1" creating pipelines={[{ id: 'p1', name: 'Sales' }]} submitLabel="Record sale" busy={false} error="" onSubmit={onSubmit} onCancel={() => undefined} />);

  it('is blank for an open deal and today again for a won one, until it is typed', async () => {
    await creating();
    const today = dateInput().props.value;
    await fireEvent.press(screen.getByRole('radio', { name: 'In progress' }));
    expect(dateInput().props.value).toBe('');
    await fireEvent.press(screen.getByRole('radio', { name: 'Won' }));
    expect(dateInput().props.value).toBe(today);
    await fireEvent.changeText(dateInput(), '2027-01-05');
    await fireEvent.press(screen.getByRole('radio', { name: 'In progress' }));
    expect(dateInput().props.value).toBe('2027-01-05');
  });

  it('opens the extra fields when the pipeline is the problem', async () => {
    await render(<DealForm workspaceId="w1" creating pipelines={[]} submitLabel="Record sale" busy={false} error="" onSubmit={() => undefined} onCancel={() => undefined} />);
    await fireEvent.changeText(screen.getByLabelText('Title'), 'Dress');
    await fireEvent.press(screen.getByRole('button', { name: 'Record sale' }));
    expect(screen.getByRole('button', { name: 'Fewer details' })).toBeTruthy();
  });
});
