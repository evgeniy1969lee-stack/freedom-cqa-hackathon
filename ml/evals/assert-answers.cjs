module.exports = (output, context) => {
  const { criteria } = JSON.parse(output);
  const predicted = Object.fromEntries(criteria.map(row => [row.criterion_id, row.answer]));
  const expected = Object.entries(context.vars.expected);
  const errors = expected.filter(([id, answer]) => predicted[id] !== answer);
  return {
    pass: errors.length === 0,
    score: (expected.length - errors.length) / expected.length,
    reason: errors.length ? errors.map(([id, answer]) =>
      `${id}: expected ${answer}, got ${predicted[id]}`).join(', ') : 'All labelled answers match',
  };
};
