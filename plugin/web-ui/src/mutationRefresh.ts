export async function mutateThenRefresh<T>(
  mutation: () => Promise<T>,
  refresh: () => Promise<unknown>,
): Promise<T> {
  const result = await mutation();
  void refresh().catch(() => undefined);
  return result;
}
